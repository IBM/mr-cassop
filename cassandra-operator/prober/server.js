/* eslint-disable no-multi-spaces */
const express = require('express')
const _ = require('lodash')
const server = express()
const httpStatus = require('http-status-codes')

const { // import pre-configured libs
  axios,
  log
} = require('./config')

require('./maintenance')
const kprober = require('./kprober')
const cassprober = require('./cassprober')

const { // initialize from environment variables
  SERVER_PORT = 8888,
  CASSANDRA_ENDPOINT_LABELS = 'app.kubernetes.io/component=database',
  LOCAL_DC_INGRESS_DOMAIN,
  PROBER_SUBDOMAIN
} = process.env

const CASSANDRA_LOCAL_SEEDS_HOSTNAMES = process.env.CASSANDRA_LOCAL_SEEDS_HOSTNAMES.split(',').map(host => host.split('.', 1)[0])
const EXTERNAL_DCS_INGRESS_DOMAINS = JSON.parse(process.env.EXTERNAL_DCS_INGRESS_DOMAINS)
const ALL_DCS_INGRESS_DOMAINS = JSON.parse(process.env.ALL_DCS_INGRESS_DOMAINS)
const LOCAL_DCS = _.map(JSON.parse(process.env.LOCAL_DCS), 'name')

log.debug('Environment Vars:', process.env)

server.get('/healthz/:broadcastip?', async (req, res) => {
  const { isReady, statesInDc } = await cassprober.processReadinessProbe(req.ip, req.params.broadcastip)
  res.status(isReady ? httpStatus.OK : httpStatus.NOT_FOUND).send(_.toString(statesInDc))
})

server.get('/readydc/:dc?', async (req, res) => {
  const localDc = req.params.dc || LOCAL_DCS[0]
  if (LOCAL_DCS.length > 1 && !req.params.dc) {
    res.status(httpStatus.BAD_REQUEST).send('[/readydc/<dc>]: <dc> must be specified if LOCAL_DCS > 1')
  } else if (LOCAL_DCS.includes(localDc)) {
    const st = (await kprober.getStatefulsetsStatus(`datacenter=${localDc},` + CASSANDRA_ENDPOINT_LABELS))[0]
    res.status(st.ready ? httpStatus.OK : httpStatus.SERVICE_UNAVAILABLE).send(_.set({}, localDc, st))
  } else {
    res.status(httpStatus.BAD_REQUEST).send(`[/readydc/<dc>] ${localDc} must exist in: ${LOCAL_DCS}`)
  }
})

server.get('/readyalldcs', (req, res) => readyDcsSlice(res))

server.get('/startdcinit/:dc', (req, res) => readyDcsSlice(res, ALL_DCS_INGRESS_DOMAINS ? LOCAL_DC_INGRESS_DOMAIN : req.params.dc))

function readyDcsSlice (res, localDc) {
  const dcs = ALL_DCS_INGRESS_DOMAINS || LOCAL_DCS
  const requests = _.takeWhile(dcs, dc => dc !== localDc) // subset of dcs before <localDc>. All dcs if localDC is undefined
    .map(dc => ALL_DCS_INGRESS_DOMAINS
      ? `${PROBER_SUBDOMAIN}.${dc}/readydc/`    // ingress-based url for external dcs
      : `:${SERVER_PORT}/readydc/${dc}`)        // url to localhost for local dcs
    .map(url => axios.get(`http://${url}`))

  Promise.allSettled(requests).then(resp => {
    const respMessage = log.debug(resp.map(responseNormalizer))
    const i = resp.findIndex(promise => promise.status === 'rejected')
    if (i >= 0) res.status(httpStatus.SERVICE_UNAVAILABLE).send(log.warn('WAITING UNREADY DCS:\n', respMessage))
    else if (localDc) res.send(log.info('OK TO START:', localDc)) // all dcs before <localDc> are ready
    else res.send(log.info('READY ALL DCS:\n', respMessage))
  })
}

function responseNormalizer (response) {
  if (response.status === 'fulfilled') return response.value?.data
  const [data, reason] = [response?.reason?.response?.data, response?.reason]
  return response.value?.data || (typeof data === 'object' ? data : { url: reason.config.url, message: reason.message })
}

const getSeedsHostIps = _ => kprober.getPodsHostIps(CASSANDRA_LOCAL_SEEDS_HOSTNAMES).then(_.compact)

server.get('/seedslocal', (req, res) => getSeedsHostIps().then(res.send)
  .catch(err => res.status(httpStatus.INTERNAL_SERVER_ERROR).send(log.error('[/seedslocal] getSeedsHostIps failed:', err))))

server.get('/seeds', (req, res) => {
  const requests = EXTERNAL_DCS_INGRESS_DOMAINS.map(url => axios.get(`https://${PROBER_SUBDOMAIN}.${url}/seedslocal`))
  Promise.allSettled([getSeedsHostIps(), ...requests]).then(responses =>
    res.send(responses.filter(r => r.status === 'fulfilled').map(r => r.value?.data || r.value).flat().join()))
})

server.get('/ping', (req, res) => res.send('pong'))

server.listen(SERVER_PORT, _ => log.info('Cassandra\'s readiness prober listening on port:', SERVER_PORT))
