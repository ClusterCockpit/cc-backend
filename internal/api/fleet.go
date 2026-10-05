// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/fleet"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	ccfleet "github.com/ClusterCockpit/cc-lib/v2/fleet"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/go-chi/chi/v5"
)

// The request and response bodies of the fleet endpoints are the wire types of
// the cc-lib fleet client (ccfleet.RegisterRequest, ccfleet.RegisterResponse),
// so client and server cannot drift apart.

// mountFleetRoutes registers the fleet service endpoints. They are
// machine-to-machine only: fleet members register, heartbeat, pull their
// configuration and deregister. Read views for the web UI are served by the
// GraphQL API, not from here.
func (api *RestAPI) mountFleetRoutes(r chi.Router) {
	r.Post("/fleet/register/cluster/", api.registerClusterService)
	r.Post("/fleet/register/infra/", api.registerInfraService)
	r.Post("/fleet/heartbeat/{instanceID}", api.fleetHeartbeat)
	r.Get("/fleet/config/{instanceID}", api.getFleetConfig)
	r.Delete("/fleet/deregister/{instanceID}", api.deregisterService)
}

// requireAPIRole enforces RoleAPI inside the handler. The AuthAPI middleware
// already gates the route group; this mirrors the nil-tolerant check the other
// machine-to-machine handlers use so it also holds with authentication disabled.
func requireAPIRole(rw http.ResponseWriter, r *http.Request) bool {
	if user := repository.GetUserFromContext(r.Context()); user != nil &&
		!user.HasRole(schema.RoleAPI) {
		handleError(fmt.Errorf("missing role: %v", schema.GetRoleString(schema.RoleAPI)),
			http.StatusForbidden, rw)
		return false
	}
	return true
}

// fleetInstanceIDLength is the hex length of an issued instance id (16 random bytes).
const fleetInstanceIDLength = 32

// validInstanceID reports whether s has the shape of an issued instance id.
// Rejecting malformed ids before the lookup keeps an id-guessing client from
// filling the error log, since the repository logs every failed lookup.
func validInstanceID(s string) bool {
	if len(s) != fleetInstanceIDLength {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// fleetInstanceID extracts and validates the instance id path parameter,
// answering 400 itself when it is malformed.
func fleetInstanceID(rw http.ResponseWriter, r *http.Request) (string, bool) {
	instanceID := chi.URLParam(r, "instanceID")
	if !validInstanceID(instanceID) {
		handleError(errors.New("malformed instance id"), http.StatusBadRequest, rw)
		return "", false
	}
	return instanceID, true
}

// validateRegistration applies the checks that map to 400. The registry returns
// unclassified errors, and hostname and cluster become path components of the
// configuration tree, so both are validated before an identity is persisted.
func validateRegistration(req *ccfleet.RegisterRequest) error {
	if req.Hostname == "" {
		return errors.New("hostname is required")
	}
	if !req.ServiceType.Valid() {
		return fmt.Errorf("unknown serviceType %q", req.ServiceType)
	}
	if err := validatePathComponent(req.Hostname, "hostname"); err != nil {
		return err
	}
	if req.Cluster != "" {
		if err := validatePathComponent(req.Cluster, "cluster"); err != nil {
			return err
		}
	}
	return nil
}

// writeRegistration answers a successful registration and asks the discovery
// publisher to emit an updated roster, so the new member is discoverable
// immediately instead of at the next publish interval.
func writeRegistration(rw http.ResponseWriter, reg *ccfleet.RegisterResponse) {
	fleet.Get().Publisher().Notify()
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(rw).Encode(reg); err != nil {
		cclog.Errorf("Failed to encode fleet registration response: %v", err)
	}
}

// registerClusterService godoc
// @summary     Register a cluster-scope fleet service
// @tags        Fleet
// @description Registers an auxiliary cc-* service that belongs to one cluster and
// @description returns the instance id it must present on every heartbeat and config pull.
// @description Re-registering the same cluster/hostname/serviceType issues a new instance id
// @description and invalidates the previous one.
// @accept      json
// @produce     json
// @param       request body     fleet.RegisterRequest true "Registration"
// @success     201     {object} fleet.RegisterResponse "Issued registration"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     403     {object} api.ErrorResponse         "Forbidden"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/fleet/register/cluster/ [post]
func (api *RestAPI) registerClusterService(rw http.ResponseWriter, r *http.Request) {
	if !requireAPIRole(rw, r) {
		return
	}

	var req ccfleet.RegisterRequest
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	if req.Cluster == "" {
		handleError(errors.New("cluster is required"), http.StatusBadRequest, rw)
		return
	}
	if err := validateRegistration(&req); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}

	reg, err := fleet.Get().Registry().Register(req)
	if err != nil {
		handleError(fmt.Errorf("registering service failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	writeRegistration(rw, reg)
}

// registerInfraService godoc
// @summary     Register a cluster-independent fleet service
// @tags        Fleet
// @description Registers an auxiliary cc-* service that is not tied to a single cluster
// @description (monitoring infrastructure) and returns its instance id.
// @accept      json
// @produce     json
// @param       request body     fleet.RegisterRequest true "Registration without cluster"
// @success     201     {object} fleet.RegisterResponse "Issued registration"
// @failure     400     {object} api.ErrorResponse         "Bad Request"
// @failure     401     {object} api.ErrorResponse         "Unauthorized"
// @failure     403     {object} api.ErrorResponse         "Forbidden"
// @failure     500     {object} api.ErrorResponse         "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/fleet/register/infra/ [post]
func (api *RestAPI) registerInfraService(rw http.ResponseWriter, r *http.Request) {
	if !requireAPIRole(rw, r) {
		return
	}

	var req ccfleet.RegisterRequest
	if err := decode(r.Body, &req); err != nil {
		handleError(fmt.Errorf("decoding request failed: %w", err), http.StatusBadRequest, rw)
		return
	}

	// The shared request body has an optional cluster; an infra service spans
	// clusters, so naming one is a client error rather than silently ignored.
	if req.Cluster != "" {
		handleError(errors.New("cluster must be empty for infra registration"), http.StatusBadRequest, rw)
		return
	}
	if err := validateRegistration(&req); err != nil {
		handleError(err, http.StatusBadRequest, rw)
		return
	}

	reg, err := fleet.Get().InfraRegistry().Register(req)
	if err != nil {
		handleError(fmt.Errorf("registering service failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	writeRegistration(rw, reg)
}

// fleetHeartbeat godoc
// @summary     Refresh liveness of a registered fleet service
// @tags        Fleet
// @description Marks the instance active. Unknown or deregistered instance ids are never
// @description recreated — the service has to register again to obtain a new instance id.
// @description Deployments running NATS should use the heartbeat subject instead.
// @produce     json
// @param       instanceID path     string true "Instance ID issued at registration"
// @success     204        "Heartbeat recorded"
// @failure     400        {object} api.ErrorResponse "Bad Request"
// @failure     401        {object} api.ErrorResponse "Unauthorized"
// @failure     403        {object} api.ErrorResponse "Forbidden"
// @failure     404        {object} api.ErrorResponse "Unknown or deregistered instance"
// @failure     500        {object} api.ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/fleet/heartbeat/{instanceID} [post]
func (api *RestAPI) fleetHeartbeat(rw http.ResponseWriter, r *http.Request) {
	if !requireAPIRole(rw, r) {
		return
	}

	instanceID, ok := fleetInstanceID(rw, r)
	if !ok {
		return
	}

	if err := fleet.Get().Registry().Heartbeat(instanceID, time.Now()); err != nil {
		if errors.Is(err, fleet.ErrUnknownInstance) {
			handleError(err, http.StatusNotFound, rw)
			return
		}
		handleError(fmt.Errorf("recording heartbeat failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// getFleetConfig godoc
// @summary     Pull the merged configuration of a fleet service
// @tags        Fleet
// @description Returns the configuration layers that apply to this instance, merged from
// @description broad to specific. The revision is a content hash served as an ETag, so a
// @description client that sends If-None-Match gets 204 (no configuration authored) or 304
// @description (unchanged) instead of the payload. 204 is not an error condition.
// @produce     json
// @param       instanceID path     string true  "Instance ID issued at registration"
// @param       If-None-Match header string false "Config revision the client already has"
// @success     200        {object} map[string]interface{} "Merged configuration"
// @success     204        "No configuration applies to this service"
// @success     304        "Configuration unchanged"
// @failure     400        {object} api.ErrorResponse "Bad Request"
// @failure     401        {object} api.ErrorResponse "Unauthorized"
// @failure     403        {object} api.ErrorResponse "Forbidden"
// @failure     404        {object} api.ErrorResponse "Unknown or deregistered instance"
// @failure     500        {object} api.ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/fleet/config/{instanceID} [get]
func (api *RestAPI) getFleetConfig(rw http.ResponseWriter, r *http.Request) {
	if !requireAPIRole(rw, r) {
		return
	}

	instanceID, ok := fleetInstanceID(rw, r)
	if !ok {
		return
	}

	res, err := fleet.Get().ResolveConfig(instanceID)
	if err != nil {
		switch {
		case errors.Is(err, fleet.ErrUnknownInstance):
			handleError(err, http.StatusNotFound, rw)
		case errors.Is(err, fleet.ErrNoConfig):
			// Not an error: no configuration has been authored for this service
			// yet, and every member polls. Answering 404 here would log a
			// warning per member per poll forever.
			rw.WriteHeader(http.StatusNoContent)
		default:
			handleError(fmt.Errorf("resolving configuration failed: %w", err), http.StatusInternalServerError, rw)
		}
		return
	}

	revision := strconv.FormatInt(res.Revision, 10)
	rw.Header().Set("ETag", `"`+revision+`"`)
	rw.Header().Set(ccfleet.HeaderConfigRevision, revision)

	// The revision is already a content hash of the merged blob, so recording it
	// only when it differs turns the steady state into a pure read.
	if res.AckedRevision != res.Revision {
		if err := fleet.Get().AckConfig(instanceID, res.Revision); err != nil {
			cclog.Warnf("fleet: acknowledging config revision %d for instance '%s' failed: %v",
				res.Revision, instanceID, err)
		}
	}

	if etagMatches(r.Header.Get("If-None-Match"), revision) {
		rw.WriteHeader(http.StatusNotModified)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write(res.Blob); err != nil {
		cclog.Errorf("Failed to write fleet config response: %v", err)
	}
}

// etagMatches reports whether an If-None-Match header covers revision. It
// accepts the wildcard, a comma separated list, weak validators and unquoted
// values, so an ordinary HTTP client works without bespoke code.
func etagMatches(header, revision string) bool {
	if header == "" {
		return false
	}
	for candidate := range strings.SplitSeq(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		candidate = strings.TrimPrefix(candidate, "W/")
		candidate = strings.Trim(candidate, `"`)
		if candidate == revision {
			return true
		}
	}
	return false
}

// deregisterService godoc
// @summary     Deregister a fleet service
// @tags        Fleet
// @description Marks the instance deregistered and drops it from the discovery rosters.
// @description Idempotent; the identity can only be restored by registering again.
// @produce     json
// @param       instanceID path     string true "Instance ID issued at registration"
// @success     204        "Service deregistered"
// @failure     400        {object} api.ErrorResponse "Bad Request"
// @failure     401        {object} api.ErrorResponse "Unauthorized"
// @failure     403        {object} api.ErrorResponse "Forbidden"
// @failure     500        {object} api.ErrorResponse "Internal Server Error"
// @security    ApiKeyAuth
// @router      /api/fleet/deregister/{instanceID} [delete]
func (api *RestAPI) deregisterService(rw http.ResponseWriter, r *http.Request) {
	if !requireAPIRole(rw, r) {
		return
	}

	instanceID, ok := fleetInstanceID(rw, r)
	if !ok {
		return
	}

	if err := fleet.Get().Registry().Deregister(instanceID); err != nil {
		handleError(fmt.Errorf("deregistering service failed: %w", err), http.StatusInternalServerError, rw)
		return
	}

	fleet.Get().Publisher().Notify()
	rw.WriteHeader(http.StatusNoContent)
}
