const loadJsonFile = require('load-json-file')
const chokidar = require('chokidar')
const Table = require('cli-table3')
const _ = require('lodash')
const dns = require('dns').promises
const httpStatus = require('http-status-codes')

const { // import pre-configured libs
  axios,
  log
} = require('./config')

const { // initialize from environment variables
  JMX_POLL_PERIOD_SECONDS = 10,
  JMX_PORT = 7199,
  JOLOKIA_PORT = 8080,
  JMX_PROXY_URL = `http://localhost:${JOLOKIA_PORT}/jolokia`,
  USERS_DIR = './users',
} = process.env

const userAuth = {
  fromFile: {},
  fallback: { user: 'cassandra', password: 'cassandra' },
  isFallback: false,
  get current () { return this.isFallback ? this.fallback : this.fromFile }
}
const nodesStates = {}
const nodesAttributes = {}

exports.processReadinessProbe = async (podIp, broadcastIp) => {
  const ip = '/' + (broadcastIp || podIp)
  if (!nodesStates[ip]) {
    nodesStates[ip] = []
    log.info(`Found a new node from readinessProbe: ${ip}`)
  }
  if (!nodesAttributes[ip]) nodesAttributes[ip] = {}
  _.assign(nodesAttributes[ip], { ip: podIp, broadcastip: broadcastIp })
  if (!nodesAttributes[ip].hostname) nodesAttributes[ip].hostname = (await dns.reverse(podIp))[0].split('.')[0]

  return {
    isReady: isNodeReady(ip),
    statesInDc: getNodeStatesInDC(ip)
  }
}

const cassRequest = {
  state: ip => jmxRequest(ip, 'org.apache.cassandra.net:type=FailureDetector', 'SimpleStates'),
  datacenter: ip => jmxRequest(ip, 'org.apache.cassandra.db:type=EndpointSnitchInfo', 'Datacenter')
}

const jmxRequest = (ip, mbean, attribute) => ({ // https://jolokia.org/features/proxy.html
  type: 'read',
  mbean: mbean,
  attribute: attribute,
  target: { url: `service:jmx:rmi:///jndi/rmi:/${ip}:${JMX_PORT}/jmxrmi`, ...userAuth.current }
})

async function updateNodesAttribute (nodeIps, attributeRequest) {
  const attribute = attributeRequest().attribute
  const noDataIPs = _.reject(nodeIps, ip => _.get(nodesAttributes, [ip, attribute]))
  const responses = await axios.post(JMX_PROXY_URL, noDataIPs.map(attributeRequest), { timeout: 0 })
  _.forEach(_.zipObject(noDataIPs, responses.data), (data, ip) => {
    if (data.status === httpStatus.OK) _.set(nodesAttributes, [ip, attribute], data.value)
    else log.warn('Failed request of attribute: ' + attribute, data.error)
  })
}

async function updateNodesRequest () {
  const polledIPs = Object.keys(nodesStates)
  const responses = await axios.post(JMX_PROXY_URL, polledIPs.map(cassRequest.state), { timeout: 0 })
  const ipsWithResponse = _(responses.data).filter({ status: httpStatus.OK }).map('value').flatMap(Object.keys).value()

  await updateNodesAttribute(ipsWithResponse, cassRequest.datacenter)

  // Create set of known knownIps by aggregating the IPs of the nodes just polled and
  // the IPs within the nodes states lists returned (with OK responses).
  // i.e. [ip1, ip2], [{ip2: UP}, {ip1: undefined, ip3: 'DOWN'}] ==> [ip1, ip2], [ip1, ip3] ==> [ip1, ip2, ip3]
  const knownIps = _.sortBy(_.union(polledIPs, ipsWithResponse), ip =>
    nodesAttributes[ip]?.hostname || nodesAttributes[ip]?.Datacenter)

  // initialize object that maps an IP to an array that will be filled with the nodes states as seen by every C* node (knownIps)
  const newNodesStates = _.fromPairs(knownIps.map(ip => [ip, Array(knownIps.length)]))
  responses.data.forEach((response, index) => {
    const ip = polledIPs[index]
    if (response.status === httpStatus.OK) {
      // map node states from response to node IPs (i.e. newNodeStates[ip] == ['UP', undefined, 'DOWN', 'UP] )
      newNodesStates[ip] = knownIps.map(ip => response.value[ip])
    } else {
      newNodesStates[ip][index] = response.status
    }
  })

  // create table object: set header and populate data rows
  const table = new Table({ head: ['ip/host', 'ready', 'dc', 'id', ..._.range(knownIps.length)] })
  _.forEach(knownIps, (ip, index) => {
    const row = {}
    row[nodesAttributes[ip]?.hostname || ip] =
      _.concat(isNodeReady(ip, newNodesStates, knownIps), nodesAttributes[ip]?.Datacenter, index, newNodesStates[ip])
    table.push(row)
  })

  // Update node states & attributes if nodes states or table changed. Table checked too in case ordering or hostnames changed.
  if (!_.isEqual(nodesStates, newNodesStates) || !_.isEqual(nodesAttributes.table, table)) {
    _.assign(nodesStates, newNodesStates) // overrides values but does not delete properties
    nodesAttributes.knownIPs = knownIps
    nodesAttributes.table = table
    log.info('STATUS MODIFIED:\nStatus Counts:', _.mapValues(nodesStates, states => _.countBy(states)))
    log.info(table.toString())
  }

  if (ipsWithResponse.length === 0) throw new Error('No updateNodeRequest was successful!')

  // Remove unreferenced nodes only if at least one updateNodeRequest was successful.
  // This is done keep retrying with last successful list of IPs and prevents emptying set all known IPs
  _.forIn(nodesStates, (states, ip) => {
    const filtered = _.without(states, undefined)
    if (filtered.length === 1 && _.isNumber(filtered[0]) && filtered[0] !== httpStatus.OK) {
      log.warn('Removing unreferenced node ip:', ip)
      _.unset(nodesStates, ip)
      _.unset(nodesAttributes, ip)
    }
  })
}

async function updateNodeStates () {
  if (_.size(nodesStates)) {
    await updateNodesRequest().catch(err => {
      log.error('Failed updateNodesRequest:\n', err, '\nRetrying with fallback auth: ', userAuth.isFallback)
      userAuth.isFallback = !userAuth.isFallback
    })
  } else log.warn('0 discovered nodes...')
}

/**
 * Determine if a Cassandra node is `ready` given an array of the `states` each cluster node indicates for it.
 * Three possible types of a `state`:
 *    - the state a Cassandra node holds for an IP (including its own)
 *    - the HTTP status error code returned from a failed request of nodes states to an IP
 *    - a JavaScript 'undefined' value when an IP does not exist in the nodes states held by a Cassandra node
 *
 * @param {String} nodeIp - the ip of the node which statuses will be compared across all DC's nodes
 * @returns {boolean} - returns true if all states equal 'UP', OR if there is one undefined state with all others 'UP'
 */
function isNodeReady (nodeIp, nStates = nodesStates, knownIPs = nodesAttributes.knownIPs) {
  const nodesInDC = getNodeStatesInDC(nodeIp, nStates, knownIPs)
  const statesCounts = _.countBy(nodesInDC) // maps each unique state to the number of its occurrences in `statesOfNode`

  return statesCounts.UP >= nodesInDC.length - 1 // (all - 1) states == UP && one undefined
}

function getNodeStatesInDC (nodeIp, nStates = nodesStates, knownIPs = nodesAttributes.knownIPs) {
  if (!nodesAttributes[nodeIp]?.Datacenter) return nStates[nodeIp]

  const nodeStateByIP = _.zipObject(knownIPs, nStates[nodeIp])
  const nodesInDC = knownIPs.filter(ip => nodesAttributes[ip]?.Datacenter === nodesAttributes[nodeIp].Datacenter)
  return _.map(nodesInDC, ip => nodeStateByIP[ip])
}

setInterval(updateNodeStates, JMX_POLL_PERIOD_SECONDS * 1000)

chokidar.watch(USERS_DIR, { alwaysStat: true, depth: 0 })
.on('all', async (event, path, stats) => {
  if (stats.isFile()) {
    const contents = await loadJsonFile(path)
    if (contents.nodetoolUser) userAuth.fromFile = { user: contents.username, password: contents.password }
  }
}).on('error', err => log.error('File watcher error:\n', err))
