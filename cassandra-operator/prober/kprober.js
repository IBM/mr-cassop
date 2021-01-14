const k8s = require('@kubernetes/client-node')
const { // import pre-configured libs
  log
} = require('./config')

const POD_NAMESPACE = process.env.POD_NAMESPACE

const kc = new k8s.KubeConfig()
kc.loadFromDefault()
const k8sApi = kc.makeApiClient(k8s.CoreV1Api)
const appsApi = kc.makeApiClient(k8s.AppsV1Api)

/**
 * @param {string[]} pods - array of Pods' names
 * @return {Promise<string|undefined[]>} - array of hostIPs/undefined matching indexes of input {@param pods}
 */
exports.getPodsHostIps = pods => Promise.allSettled(pods.map(getPod)).then(promises => promises.map(p => p.value?.status?.hostIP))

/**
 * @param {string} [podName] - name of the Pod object to return. Should match the Pod's 'metadata.name'.
 * @returns {Promise<V1Pod>}
 */
const getPod = podName => k8sApi.readNamespacedPod(podName, POD_NAMESPACE).then(res => res.body)

/**
 * @param {string} [labelSelector=''] - selector to restrict the list of returned objects by their labels. Defaults to everything.
 * @returns {Promise<Array<V1StatefulSet>>}
 */
const getStatefulsets = labelSelector =>
  appsApi.listNamespacedStatefulSet(POD_NAMESPACE, undefined, undefined, undefined, undefined, labelSelector)
  .then(res => res.body.items).catch(log.error)

/**
 * @param {V1StatefulSet} statefulSet
 * @return {{           replicas: number,
 *                      readyReplicas: number
 *            readonly  ready: boolean }}
 */
const getStatefulsetStatus = statefulSet => ({
  replicas: statefulSet.spec.replicas || 0,
  readyReplicas: statefulSet.status.readyReplicas || 0,
  get ready () { return this.replicas === this.readyReplicas }
})

/**
 * @param {string} labelSelector
 * @return {Promise<{           replicas: number,
 *                              readyReplicas: number,
 *                     readonly  ready: boolean }[]> }
 */
exports.getStatefulsetsStatus = labelSelector => getStatefulsets(labelSelector).then(sts => sts.map(getStatefulsetStatus))
