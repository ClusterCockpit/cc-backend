// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package natsMessenger

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/importer"
	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/golang-jwt/jwt/v5"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type NatsMessenger struct {
	Server        *server.Server
	Connection    *nats.Conn
	Subscriptions []*nats.Subscription
	JobRepository *repository.JobRepository
	jwtPubKey     ed25519.PublicKey
}

var natsMessengerInstance *NatsMessenger
var once sync.Once

type DevNatsMessage struct {
	Content string `json:"content"`
}

// StartJobNatsResponse model
type StartJobNatsResponse struct {
	// Database ID of new job
	DBID int64 `json:"id"`
}

// StopJobNatsRequest model
type StopJobNatsRequest struct {
	JobId     *int64          `json:"jobId" example:"123000"`
	Cluster   *string         `json:"cluster" example:"fritz"`
	StartTime *int64          `json:"startTime" example:"1649723812"`
	State     schema.JobState `json:"jobState" validate:"required" example:"completed"`
	StopTime  int64           `json:"stopTime" validate:"required" example:"1649763839"`
}

// DeleteJobNatsRequest model
type DeleteJobNatsRequest struct {
	JobId     *int64  `json:"jobId" validate:"required" example:"123000"` // Cluster Job ID of job
	Cluster   *string `json:"cluster" example:"fritz"`                    // Cluster of job
	StartTime *int64  `json:"startTime" example:"1649723812"`             // Start Time of job as epoch
}

// jobEventNatsRequest model
type ReceiveEventNatsRequest struct {
	JobId     *int64  `json:"jobId" validate:"required" example:"123000"` // Cluster Job ID of job
	Cluster   *string `json:"cluster" example:"fritz"`                    // Cluster of job
	StartTime *int64  `json:"startTime" example:"1649723812"`             // Start Time of job as epoch
	Metric    *string `json:"metric" example:"cpu_power"`                 // Event Target Metric for Job
	Timestamp *int64  `json:"timestamp" example:"1649724000"`             // Event Timestamp
	Event     *string `json:"event" example:"powercap"`                   // Event Name / Type
	Value     *int64  `json:"value,omitempty" example:"150"`              // Optional Value Set for Evenr, eg powercap
}

// Get Singleton
func GetNatsMessenger(config *schema.NatsConfig) *NatsMessenger {
	// Check if Config present
	if config == nil {
		log.Info("No NATS config found: Skip NATS init.")
		return nil
	}

	if natsMessengerInstance == nil {
		once.Do(
			func() {
				// Raw Init
				var err error
				natsMessengerInstance = &NatsMessenger{
					Server:        nil,
					Connection:    nil,
					Subscriptions: []*nats.Subscription{},
					JobRepository: repository.GetJobRepository(),
					jwtPubKey:     nil,
				}
				// Init JWT PubKey
				pubKey := os.Getenv("JWT_PUBLIC_KEY")
				if pubKey == "" {
					log.Warn("environment variable 'JWT_PUBLIC_KEY' not set (token based authentication will not work for nats: abort setup)")
				} else {
					if bytes, err := base64.StdEncoding.DecodeString(pubKey); err != nil {
						log.Warn("Could not decode JWT public key")
					} else {
						natsMessengerInstance.jwtPubKey = ed25519.PublicKey(bytes)
					}
				}

				// Start Nats Server
				// Note: You can configure things like Host, Port, Authorization, and much more using server.Options.
				opts := &server.Options{Port: config.Port}
				if natsMessengerInstance.Server, err = server.NewServer(opts); err != nil {
					log.Error("nats server error on creation")
				}

				go natsMessengerInstance.Server.Start()

				if !natsMessengerInstance.Server.ReadyForConnections(3 * time.Second) {
					log.Error("nats server not ready for connection")
				}

				// Connect
				var copts []nats.Option
				if natsMessengerInstance.Connection, err = nats.Connect(natsMessengerInstance.Server.ClientURL(), copts...); err != nil {
					natsMessengerInstance.Server.Shutdown()
					log.Error("nats connection could not be established: nats shut down")
				}

				// Subscribe
				if err = natsMessengerInstance.setupSubscriptions(); err != nil {
					log.Error("error when subscribing to channels: nats shut down")
					natsMessengerInstance.Connection.Close()
					natsMessengerInstance.Server.Shutdown()
				}
			})
		log.Infof("NATS server and subscriptions on port '%d' established\n", config.Port)
	} else {
		log.Infof("Single NatsMessenger instance already created on port '%d'\n", config.Port)
	}

	return natsMessengerInstance
}

func (nm *NatsMessenger) StopNatsMessenger() {
	for _, sub := range nm.Subscriptions {
		err := sub.Unsubscribe()
		if err != nil {
			log.Errorf("NATS unsubscribe failed: %s", err.Error())
		}
		sub.Drain()
	}

	nm.Connection.Close()
	nm.Server.Shutdown()
	log.Info("NATS connections closed and server shut down")
}

func (nm *NatsMessenger) setupSubscriptions() (err error) {

	if startSub, err := nm.startJobListener(); err != nil {
		log.Infof("Subscription to 'start-job' failed: %s", err)
	} else {
		log.Info("Subscribed to 'start-job'")
		nm.Subscriptions = append(nm.Subscriptions, startSub)
	}

	if stopSub, err := nm.stopJobListener(); err != nil {
		log.Infof("Subscription to 'stop-job' failed: %s", err)
	} else {
		log.Info("Subscribed to 'stop-job'")
		nm.Subscriptions = append(nm.Subscriptions, stopSub)
	}

	if deleteSub, err := nm.deleteJobListener(); err != nil {
		log.Infof("Subscription to 'delete-job' failed: %s", err)
	} else {
		log.Info("Subscribed to 'delete-job'")
		nm.Subscriptions = append(nm.Subscriptions, deleteSub)
	}

	if eventSub, err := nm.jobEventListener(); err != nil {
		log.Infof("Subscription to 'job-event' failed: %s", err)
	} else {
		log.Info("Subscribed to 'job-event'")
		nm.Subscriptions = append(nm.Subscriptions, eventSub)
	}

	return err
}

// Listeners: Subscribe to specified channels and handle with specific handler functions

func (nm *NatsMessenger) startJobListener() (sub *nats.Subscription, err error) {
	return nm.Connection.Subscribe("start-job", func(m *nats.Msg) {
		user, err := nm.verifyMessageJWT(m)

		if err != nil {
			log.Warnf("not authd: %s", err.Error())
			m.Respond([]byte("not authd: " + err.Error()))
		} else if user != nil && user.HasRole(schema.RoleApi) {
			req := schema.JobMeta{BaseJob: schema.JobDefaults}
			if err := json.Unmarshal(m.Data, &req); err != nil {
				log.Warnf("Error while unmarshaling raw json nats message content on channel start-job: %s", err.Error())
				m.Respond([]byte("Error while unmarshaling raw json nats message content on channel start-job: " + err.Error()))
			}
			m.Respond(nm.startJobHandler(req))
		} else {
			log.Warnf("missing role for nats")
			m.Respond([]byte("missing role for nats"))
		}
	})
}

func (nm *NatsMessenger) stopJobListener() (sub *nats.Subscription, err error) {
	return nm.Connection.Subscribe("stop-job", func(m *nats.Msg) {
		user, err := nm.verifyMessageJWT(m)

		if err != nil {
			log.Warnf("not authd: %s", err.Error())
			m.Respond([]byte("not authd: " + err.Error()))
		} else if user != nil && user.HasRole(schema.RoleApi) {
			var req StopJobNatsRequest
			if err := json.Unmarshal(m.Data, &req); err != nil {
				log.Error("Error while unmarshaling raw json nats message content: stopJob")
				m.Respond([]byte("Error while unmarshaling raw json nats message content: stopJob"))
			}
			m.Respond(nm.stopJobHandler(req))
		} else {
			log.Warnf("missing role for nats")
			m.Respond([]byte("missing role for nats"))
		}
	})
}

func (nm *NatsMessenger) deleteJobListener() (sub *nats.Subscription, err error) {
	return nm.Connection.Subscribe("delete-job", func(m *nats.Msg) {
		var req DevNatsMessage
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Error("Error while unmarshaling raw json nats message content: deleteJob")
		}

		if err := nm.deleteJobHandler(req); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})
}

func (nm *NatsMessenger) jobEventListener() (sub *nats.Subscription, err error) {
	return nm.Connection.Subscribe("job-event", func(m *nats.Msg) {
		var req DevNatsMessage
		if err := json.Unmarshal(m.Data, &req); err != nil {
			log.Error("Error while unmarshaling raw json nats message content: jobEvent")
		}

		if err := nm.jobEventHandler(req); err != nil {
			log.Errorf("error: %s", err.Error())
		}
	})
}

// Handlers: Take content of message and perform action, e.g. adding job in db

func (nm *NatsMessenger) startJobHandler(req schema.JobMeta) []byte {
	if req.State == "" {
		req.State = schema.JobStateRunning
	}
	if err := importer.SanityChecks(&req.BaseJob); err != nil {
		log.Error(err)
		return handleErr(err)
	}

	// // aquire lock to avoid race condition between API calls --> for NATS required?
	// var unlockOnce sync.Once
	// api.RepositoryMutex.Lock()
	// defer unlockOnce.Do(api.RepositoryMutex.Unlock)

	// Check if combination of (job_id, cluster_id, start_time) already exists:
	jobs, err := nm.JobRepository.FindAll(&req.JobID, &req.Cluster, nil)
	if err != nil && err != sql.ErrNoRows {
		log.Errorf("checking for duplicate failed: %s", err)
		return handleErr(fmt.Errorf("checking for duplicate failed: %w", err))
	} else if err == nil {
		for _, job := range jobs {
			if (req.StartTime - job.StartTimeUnix) < 86400 {
				log.Errorf("a job with that jobId, cluster and startTime already exists: dbid: %d, jobid: %d", job.ID, job.JobID)
				return handleErr(fmt.Errorf("a job with that jobId, cluster and startTime already exists: dbid: %d, jobid: %d", job.ID, job.JobID))
			}
		}
	}

	// id, err := nm.JobRepository.Start(&req)
	// if err != nil {
	// 	log.Errorf("insert into database failed: %s", err)
	// 	return handleErr(fmt.Errorf("insert into database failed: %w", err))
	// }

	// // unlock here, adding Tags can be async
	// unlockOnce.Do(api.RepositoryMutex.Unlock)

	for _, tag := range req.Tags {
		if _, err := nm.JobRepository.AddTagOrCreate(1337, tag.Type, tag.Name); err != nil {
			log.Errorf("adding tag to new job %d failed: %s", 1337, err)
			return handleErr(fmt.Errorf("adding tag to new job %d failed: %w", 1337, err))
		}
	}

	log.Infof("new job (id: %d): cluster=%s, jobId=%d, user=%s, startTime=%d", 1337, req.Cluster, req.JobID, req.User, req.StartTime)

	result, _ := json.Marshal(StartJobNatsResponse{
		DBID: 1337,
	})
	return result
}

func (nm *NatsMessenger) stopJobHandler(req StopJobNatsRequest) []byte {
	// Fetch job (that will be stopped) from db
	var job *schema.Job
	var err error
	if req.JobId == nil {
		return handleErr(errors.New("the field 'jobId' is required"))
	}

	job, err = nm.JobRepository.Find(req.JobId, req.Cluster, req.StartTime)
	if err != nil {
		return handleErr(fmt.Errorf("finding job failed: %w", err))
	}

	// Sanity checks
	if job == nil || job.StartTime.Unix() >= req.StopTime || job.State != schema.JobStateRunning {
		return handleErr(errors.New("stopTime must be larger than startTime and only running jobs can be stopped"))
	}

	if req.State != "" && !req.State.Valid() {
		return handleErr(fmt.Errorf("invalid job state: %#v", req.State))
	} else if req.State == "" {
		req.State = schema.JobStateCompleted
	}

	// Mark job as stopped in the database (update state and duration)
	job.Duration = int32(req.StopTime - job.StartTime.Unix())
	job.State = req.State
	// if err := nm.JobRepository.Stop(job.ID, job.Duration, job.State, job.MonitoringStatus); err != nil {
	// 	return handleErr(fmt.Errorf("marking job as stopped failed: %w", err))
	// }

	log.Infof("archiving job... (dbid: %d): cluster=%s, jobId=%d, user=%s, startTime=%s", job.ID, job.Cluster, job.JobID, job.User, job.StartTime)

	// // Send a response (with status OK). This means that erros that happen from here on forward
	// // can *NOT* be communicated to the client. If reading from a MetricDataRepository or
	// // writing to the filesystem fails, the client will not know.
	// rw.Header().Add("Content-Type", "application/json")
	// rw.WriteHeader(http.StatusOK)
	// json.NewEncoder(rw).Encode(job)

	// Monitoring is disabled...
	if job.MonitoringStatus == schema.MonitoringStatusDisabled {
		return handleErr(fmt.Errorf("monitoring is disabled"))
	}

	// Trigger async archiving
	// nm.JobRepository.TriggerArchiving(job)

	result, _ := json.Marshal(job)
	return result
}

func (nm *NatsMessenger) deleteJobHandler(req DevNatsMessage) (err error) {
	// Allow via Nats?
	log.Debugf("CALLED HANDLER FOR deleteJob: %s", req.Content)
	return nil
}

func (nm *NatsMessenger) jobEventHandler(req DevNatsMessage) (err error) {
	// Implement from scratch
	log.Debugf("CALLED HANDLER FOR jobEvent: %s", req.Content)
	return nil
}

// Auth

func (nm *NatsMessenger) verifyMessageJWT(msg *nats.Msg) (user *schema.User, err error) {

	var rawtoken string
	if rawtoken = msg.Header.Get("auth"); rawtoken == "" {
		return nil, errors.New("missing token")
	}

	token, err := jwt.Parse(rawtoken, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("only Ed25519/EdDSA supported")
		}

		return nm.jwtPubKey, nil
	})
	if err != nil {
		log.Warn("Error while parsing JWT token")
		return nil, err
	}
	if !token.Valid {
		log.Warn("jwt token claims are not valid")
		return nil, errors.New("jwt token claims are not valid")
	}

	// Token is valid, extract payload
	claims := token.Claims.(jwt.MapClaims)
	sub, _ := claims["sub"].(string)

	// NATS: Always Validate user + roles from JWT against database
	ur := repository.GetUserRepository()
	user, err = ur.GetUser(sub)
	// Deny any logins for unknown usernames
	if err != nil {
		log.Warn("Could not find user from JWT in internal database.")
		return nil, errors.New("unknown user")
	}

	return &schema.User{
		Username:   sub,
		Roles:      user.Roles, // Take user roles from database instead of trusting the JWT
		AuthType:   schema.AuthToken,
		AuthSource: -1,
	}, nil
}

// Helper

func handleErr(err error) []byte {
	res, _ := json.Marshal(err.Error())
	return res
}
