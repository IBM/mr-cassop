const k8s = require('@kubernetes/client-node')
const express = require('express')
const server = express()

server.use(express.json())

const {
  log
} = require('./config')

const kc = new k8s.KubeConfig()
kc.loadFromDefault()
const k8sApi = kc.makeApiClient(k8s.CoreV1Api)

const {
  CONFIGMAP_NAME,
  POD_NAMESPACE,
  MAINTENANCE_PORT,
  CASSANDRA_ENDPOINT_LABELS = 'app.kubernetes.io/component=database'
} = process.env

const options = {
  headers: {
    "Content-Type": "application/merge-patch+json"
  }
}

// true = maintenance mode, false = normal mode

/**
 * Updates the maintenance mode state for the selected pod in the ConfigMap.
 *
 * @param {boolean} requestedMode the maintenance mode - true to activate or false to disable
 * @param {Array<String>} pods the name of the pod(s)
 * @param {String} [configMap] the name of the ConfigMap
 * @param {String} [namespace] the pod namespace
 * @returns {Promise} the response from the k8s API
 */
async function updateConfigMap(requestedMode, pods, configMap = CONFIGMAP_NAME, namespace = POD_NAMESPACE) {
  const map = await getConfigMap(configMap, namespace).catch((err) => Promise.reject(`Error getting config map: ${err.message}`))
  const inMaintenance = typeof map.body.data !== 'undefined' ? map.body.data : {}
  pods.forEach((p) => {
    requestedMode ? inMaintenance[p] = "true" : inMaintenance[p] = null
  })
  const body = {
    data: inMaintenance
  }
  return k8sApi.patchNamespacedConfigMap(configMap, namespace, body, undefined, undefined, undefined, undefined, options)
}

/**
 * Updates the phase of the pod to "Failed" to force a restart.
 *
 * @param {String} pod the name of the pod
 * @param {String} [namespace] the pod namespace
 * @returns {Promise} the response from the k8s API
 */
function updatePodStatus(pod, namespace = POD_NAMESPACE) {
  const body = {
    status: {
      phase: "Failed"
    }
  }
  return k8sApi.patchNamespacedPodStatus(pod, namespace, body, undefined, undefined, undefined, undefined, options)
}

/**
 * Updates the pod status of all pods in the list.
 *
 * @param {boolean} mode the maintenance mode
 * @param {Array<String>} pods the list of pods
 * @returns {Array<Promise>}
 */
async function updatePodStatuses(mode, pods) {
  const statuses = []
  for (let i = 0; i < pods.length; i++) {
    const name = pods[i]
    if (mode && await checkCassandraRunning(name)) {
      statuses.push(await updatePodStatus(name))
    } else {
      statuses.push(getPodStatus(name))
    }
  }
  return statuses
}

/**
 * Gets the maintenance ConfigMap.
 *
 * @param {String} [configMap] the name of the ConfigMap
 * @param {String} [namespace] the namespace of the ConfigMap
 * @returns {Promise} the ConfigMap
 */
function getConfigMap(configMap = CONFIGMAP_NAME, namespace = POD_NAMESPACE) {
  return k8sApi.readNamespacedConfigMap(configMap, namespace)
}

/**
 * Gets the status of the specified pod.
 *
 * @param {String} pod the name of the pod
 * @param {String} [namespace] the pod namespace
 * @returns {Promise} the Pod
 */
function getPodStatus(pod, namespace = POD_NAMESPACE) {
  return k8sApi.readNamespacedPodStatus(pod, namespace)
}

/**
 * Checks if the maintenance-mode initContainer is running.
 *
 * @param {String} pod the name of the pod
 * @param {String} namespace the pod namespace
 * @returns {Promise<boolean>} whether or not the pod is in maintenance mode
 */
async function checkMaintenanceRunning(pod, namespace = POD_NAMESPACE) {
  const res = await getPodStatus(pod, namespace).catch((err) => Promise.reject(err))
  const initContainers = res.body.status.initContainerStatuses
  let isRunning = false
  if (initContainers.length > 0) {
    initContainers.forEach((container) => {
      if (container.name === 'maintenance-mode' && container.state.running) {
        log.debug(`Maintenance mode container is running in pod ${pod}`)
        isRunning = true
      }
    })
  }
  return isRunning
}

/**
 * Checks if the Cassandra container is running.
 *
 * @param {String} pod the name of the pod
 * @param {String} namespace the pod namespace
 * @returns {Promise<boolean>} whether or not the C* container is running
 */
async function checkCassandraRunning(pod, namespace = POD_NAMESPACE) {
  const res = await getPodStatus(pod, namespace).catch((err) => Promise.reject(err))
  const containerStatuses = res.body.status.containerStatuses
  let isRunning = false
  if (containerStatuses.length > 0) {
    containerStatuses.forEach((container) => {
      if (container.name === 'cassandra' && container.state.running) {
        log.debug(`Cassandra container is running in pod ${pod}`)
        isRunning = true
      }
    })
  }
  return isRunning
}

/**
 * Formats a list of Promise.allSettled responses from the Kubernetes API.
 *
 * @param {boolean} mode the maintenance mode
 * @param {[Promise]} responses the list of responses to format
 * @returns {Array} the formatted list of responses
 */
function formatResponse(mode, responses) {
  return responses.map(r => {
    if (r.value && r.value.response) {
      return { [r.value.response.body.metadata.name]: mode }
    }
    return { error: r.reason }
  })
}

/**
 * Gets the maintenance mode of the selected pod.
 *
 * @param {Request} req the request object
 * @param {Response} res the response object
 */
async function getPodMaintenanceMode(req, res) {
  const pod = req.params.pod
  try {
    const status = await checkMaintenanceRunning(pod)
    res.json({ inMaintenance: status })
  } catch(err) {
    log.error(`Failed to get status of pod ${pod}: ${err.message}`)
    res.status(404).send(`Pod ${pod} does not exist.`)
  }
}

/**
 * Updates the maintenance mode of the selected pod.
 *
 * @param {boolean} mode the maintenance mode
 * @param {Request} req the request object
 * @param {Response} res the response object
 */
async function updatePodMaintenanceMode(mode, req, res) {
  const pod = req.params.pod
  const status = await getPodStatus(pod).catch((err) => {
    log.error(`Pod ${pod} does not exist.`)
    res.status(404).send(`Pod ${pod} does not exist: ${err.message}`)
  })
  try {
    if (status) {
      await updateConfigMap(mode, [pod])
      if (mode && await checkCassandraRunning(pod)) await updatePodStatus(pod)
      res.send({ [pod]: mode })
    }
  } catch (err) {
    log.error(`Failed to update pod maintenance mode for pod ${pod}.`)
    res.status(503).send(`Failed to update maintenance mode for pod ${pod}: ${err.message}`)
  }
}

/**
 * Gets the maintenance mode of the selected dc.
 *
 * @param {Request} req the request object
 * @param {Response} res the response object
 */
async function getDcMaintenanceMode(req, res) {
  const dc = req.params.dc
  try {
    const matches = await filterMatches(dc)
    if (matches.length > 0) {
      matches.sort(sortByDc)
      const map = await getConfigMap()
      const inMaintenance = typeof map.body.data !== 'undefined' ? map.body.data : null
      let mode = false
      if (inMaintenance != null) {
        const result = matches.filter(v => Object.keys(inMaintenance).includes(v))
        mode = result.length === matches.length
      }
      res.json({ [dc]: mode })
    } else {
      res.status(404).send(`No matches found for dc ${dc}.`)
    }
  } catch(err) {
    log.error(`Failed to get maintenance mode for dc ${dc}.`)
    res.status(503).send(`Failed to get maintenance mode for dc ${dc}: ${err.message}`)
  }
}

/**
 * Updates the maintenance mode of a data center.
 *
 * @param {boolean} mode the maintenance mode
 * @param {Request} req the request object
 * @param {Response} res the response object
 */
async function updateDcMaintenanceMode(mode, req, res) {
  const dc = req.params.dc
  try {
    const matches = await filterMatches(dc)
    if (matches.length > 0) {
      matches.sort(sortByDc)
      await updateConfigMap(mode, matches)
      const statuses = await updatePodStatuses(mode, matches)
      Promise.allSettled(statuses).then(responses => res.json(formatResponse(mode, responses)))
    } else {
      res.status(404).send(`No matches found for dc ${dc}.`)
    }
  } catch(err) {
    log.error(`Failed to update maintenance mode for dc ${dc}.`)
    res.status(503).send(`Failed to update maintenance mode for dc ${dc}: ${err.message}`)
  }
}

/**
 * Sorts by the replica number at the end of the pod name.
 *
 * @param {String} a the first pod
 * @param {String} b the second pod
 * @returns {number}
 */
function sortByDc(a, b) {
  return a.substring(a.lastIndexOf("-")) - b.substring(b.lastIndexOf("-"))
}

/**
 * Filters pods by datacenter.
 *
 * @param {String} dc the datacenter
 * @returns {Array}
 */
async function filterMatches(dc) {
  const pods = await k8sApi.listNamespacedPod(POD_NAMESPACE, undefined, undefined, undefined, undefined, CASSANDRA_ENDPOINT_LABELS)
  const names = pods.body.items.map((e) => e.metadata.name)
  return names.filter((name) => name.includes(dc))
}

/**
 * Gets the ConfigMap object.
 */
server.get('/config', async (req, res) => {
  res.json(await getConfigMap().catch((err) => {
    log.error("Error getting ConfigMap.")
    res.status(500).send(`${err.message}: error getting ConfigMap`)
  }))
})

/**
 * Gets the maintenance status of a given pod.
 */
server.get('/pods/:pod', async (req, res) => {
  await getPodMaintenanceMode(req, res)
})


/**
 * Activates maintenance mode for a given pod.
 */
server.put('/pods/:pod', async (req, res) => {
  await updatePodMaintenanceMode(true, req, res)
})

/**
 * Disables maintenance mode for a given pod.
 */
server.delete('/pods/:pod', async (req, res) => {
  await updatePodMaintenanceMode(false, req, res)
})

server.get('/dcs/:dc', async (req, res) => {
  await getDcMaintenanceMode(req, res)
})

/**
 * Activates maintenance mode for all pods in a data center.
 */
server.put('/dcs/:dc', async (req, res) => {
  await updateDcMaintenanceMode(true, req, res)
})

/**
 * Disables maintenance mode for all pods in a data center.
 */
server.delete('/dcs/:dc', async (req, res) => {
  await updateDcMaintenanceMode(false, req, res)
})

server.listen(MAINTENANCE_PORT, '0.0.0.0', () => log.info('Cassandra maintenance monitor listening on port:', MAINTENANCE_PORT))
