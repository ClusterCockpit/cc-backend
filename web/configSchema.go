// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package web

const configSchema = `{
  "type": "object",
  "properties": {
    "jobList": {
      "description": "Job list defaults. Applies to user- and jobs views.",
      "type": "object",
      "properties": {
        "usePaging": {
          "description": "If classic paging is used instead of continuous scrolling by default.",
          "type": "boolean"
        },
        "showFootprint": {
          "description": "If footprint bars are shown as first column by default.",
          "type": "boolean"
        }
      }
    },
    "nodeList": {
      "description": "Node list defaults. Applies to node list view.",
      "type": "object",
      "properties": {
        "usePaging": {
          "description": "If classic paging is used instead of continuous scrolling by default.",
          "type": "boolean"
        }
      }
    },
    "jobView": {
      "description": "Job view defaults.",
      "type": "object",
      "properties": {
        "showPolarPlot": {
          "description": "If the job metric footprints polar plot is shown by default.",
          "type": "boolean"
        },
        "showFootprint": {
          "description": "If the annotated job metric footprint bars are shown by default.",
          "type": "boolean"
        },
        "showRoofline": {
          "description": "If the job roofline plot is shown by default.",
          "type": "boolean"
        },
        "showStatTable": {
          "description": "If the job metric statistics table is shown by default.",
          "type": "boolean"
        }
      }
    },
    "metricConfig": {
      "description": "Global initial metric selections for primary views of all clusters.",
      "type": "object",
      "properties": {
        "jobListMetrics": {
          "description": "Initial metrics shown for new users in job lists (User and jobs view).",
          "type": "array",
          "items": {
            "type": "string",
            "minItems": 1
          }
        },
        "jobViewPlotMetrics": {
          "description": "Initial metrics shown for new users as job view metric plots.",
          "type": "array",
          "items": {
            "type": "string",
            "minItems": 1
          }
        },
        "jobViewTableMetrics": {
          "description": "Initial metrics shown for new users in job view statistics table.",
          "type": "array",
          "items": {
            "type": "string",
            "minItems": 1
          }
        },
        "clusters": {
          "description": "Overrides for global defaults by cluster and subcluster.",
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "name": {
                "description": "The name of the cluster."
              },
              "jobListMetrics": {
                "description": "Initial metrics shown for new users in job lists (User and jobs view) for subcluster.",
                "type": "array",
                "items": {
                  "type": "string",
                  "minItems": 1
                }
              },
              "jobViewPlotMetrics": {
                "description": "Initial metrics shown for new users as job view timeplots for subcluster.",
                "type": "array",
                "items": {
                  "type": "string",
                  "minItems": 1
                }
              },
              "jobViewTableMetrics": {
                "description": "Initial metrics shown for new users in job view statistics table for subcluster.",
                "type": "array",
                "items": {
                  "type": "string",
                  "minItems": 1
                }
              },
              "subClusters": {
                "description": "The array of overrides per subcluster.",
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "name": {
                      "description": "The name of the subcluster.",
                      "type": "string"
                    },
                    "jobListMetrics": {
                      "description": "Initial metrics shown for new users in job lists (User and jobs view) for subcluster.",
                      "type": "array",
                      "items": {
                        "type": "string",
                        "minItems": 1
                      }
                    },
                    "jobViewPlotMetrics": {
                      "description": "Initial metrics shown for new users as job view timeplots for subcluster.",
                      "type": "array",
                      "items": {
                        "type": "string",
                        "minItems": 1
                      }
                    },
                    "jobViewTableMetrics": {
                      "description": "Initial metrics shown for new users in job view statistics table for subcluster.",
                      "type": "array",
                      "items": {
                        "type": "string",
                        "minItems": 1
                      }
                    }
                  },
                  "required": ["name"],
                  "minItems": 1
                }
              }
            },
            "required": ["name", "subClusters"],
            "minItems": 1
          }
        }
      }
    },
    "plotConfiguration": {
      "description": "Initial settings for plot render options.",
      "type": "object",
      "properties": {
        "colorBackground": {
          "description": "If the metric plot backgrounds are initially colored by threshold limits.",
          "type": "boolean"
        },
        "plotsPerRow": {
          "description": "How many plots are initially rendered in per row. Applies to job, single node, and analysis views.",
          "type": "integer"
        },
        "lineWidth": {
          "description": "Initial thickness of rendered plotlines. Applies to metric plot, job compare plot and roofline.",
          "type": "integer"
        },
        "colorScheme": {
          "description": "Initial colorScheme to be used for metric plots.",
          "type": "array",
          "items": {
            "type": "string"
          }
        }
      }
    }
  }
}`
