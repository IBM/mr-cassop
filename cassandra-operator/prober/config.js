const _ = require('lodash')
const { // initialize from environment variables
  LOG_LEVEL // info|debug|trace
} = process.env

const axios = require('axios').default
axios.defaults.timeout = 2000 // milliseconds
if (LOG_LEVEL === 'trace') {
  const AxiosLogger = require('axios-logger')
  axios.interceptors.request.use(AxiosLogger.requestLogger, AxiosLogger.errorLogger)
  axios.interceptors.response.use(AxiosLogger.responseLogger, AxiosLogger.errorLogger)
}

const log = require('ololog')
.configure({
  time: true,
  tag: true,
  locate: { shift: 1 },
  returnValue: (formatted, { initialArguments }) => _.size(initialArguments) === 1 ? initialArguments[0] : formatted
}).methods({
  get debug () {
    return this.configure(
      { tag: { level: 'debug' }, render: ['debug', 'trace'].includes(LOG_LEVEL) ? { consoleMethod: 'debug' } : false })
  },
  handleUncaughtErrors () {
    process.on('uncaughtException', this.bright.red.error)
    process.on('unhandledRejection', this.bright.red.error)
    return this
  }
}).handleUncaughtErrors()

module.exports = { axios, log }
