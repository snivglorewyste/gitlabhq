import { Configuration, WatchApi, EVENT_DATA } from '@gitlab/cluster-client';
import axios from '~/lib/utils/axios_utils';
import {
  HELM_RELEASES_RESOURCE_TYPE,
  KUSTOMIZATIONS_RESOURCE_TYPE,
} from '~/environments/constants';
import fluxKustomizationStatusQuery from '../queries/flux_kustomization_status.query.graphql';
import fluxHelmReleaseStatusQuery from '../queries/flux_helm_release_status.query.graphql';

const helmReleasesApiVersion = 'helm.toolkit.fluxcd.io/v2beta1';
const kustomizationsApiVersion = 'kustomize.toolkit.fluxcd.io/v1beta1';

const helmReleaseField = 'fluxHelmReleaseStatus';
const kustomizationField = 'fluxKustomizationStatus';

const handleClusterError = (err) => {
  const error = err?.response?.data?.message ? new Error(err.response.data.message) : err;
  throw error;
};

const buildFluxResourceUrl = ({
  basePath,
  namespace,
  apiVersion,
  resourceType,
  environmentName = '',
}) => {
  return `${basePath}/apis/${apiVersion}/namespaces/${namespace}/${resourceType}/${environmentName}`;
};

const buildFluxResourceWatchPath = ({ namespace, apiVersion, resourceType }) => {
  return `/apis/${apiVersion}/namespaces/${namespace}/${resourceType}`;
};

const watchFluxResource = ({ watchPath, resourceName, query, variables, field, client }) => {
  const config = new Configuration(variables.configuration);
  const watcherApi = new WatchApi(config);
  const fieldSelector = `metadata.name=${decodeURIComponent(resourceName)}`;

  watcherApi
    .subscribeToStream(watchPath, { watch: true, fieldSelector })
    .then((watcher) => {
      let result = [];

      watcher.on(EVENT_DATA, (data) => {
        result = data[0]?.status?.conditions;

        client.writeQuery({
          query,
          variables,
          data: { [field]: result },
        });
      });
    })
    .catch((err) => {
      handleClusterError(err);
    });
};

const getFluxResourceStatus = ({ url, watchPath, query, variables, field, client }) => {
  const { headers } = variables.configuration;
  const withCredentials = true;

  return axios
    .get(url, { withCredentials, headers })
    .then((res) => {
      const fluxData = res?.data;
      const resourceName = fluxData?.metadata?.name;

      if (gon.features?.k8sWatchApi && resourceName) {
        watchFluxResource({
          watchPath,
          resourceName,
          query,
          variables,
          field,
          client,
        });
      }

      return fluxData?.status?.conditions || [];
    })
    .catch((err) => {
      handleClusterError(err);
    });
};

const getFluxResources = (configuration, url) => {
  const { headers } = configuration;
  const withCredentials = true;

  return axios
    .get(url, { withCredentials, headers })
    .then((res) => {
      const items = res?.data?.items || [];
      const result = items.map((item) => {
        return {
          apiVersion: item.apiVersion,
          metadata: {
            name: item.metadata?.name,
            namespace: item.metadata?.namespace,
          },
        };
      });
      return result || [];
    })
    .catch((err) => {
      const error = err?.response?.data?.reason || err;
      throw new Error(error);
    });
};

export default {
  fluxKustomizationStatus(
    _,
    { configuration, namespace, environmentName, fluxResourcePath = '' },
    { client },
  ) {
    const watchPath = buildFluxResourceWatchPath({
      namespace,
      apiVersion: kustomizationsApiVersion,
      resourceType: KUSTOMIZATIONS_RESOURCE_TYPE,
    });
    let url;

    if (fluxResourcePath) {
      url = `${configuration.basePath}/apis/${fluxResourcePath}`;
    } else {
      url = buildFluxResourceUrl({
        basePath: configuration.basePath,
        resourceType: KUSTOMIZATIONS_RESOURCE_TYPE,
        apiVersion: kustomizationsApiVersion,
        namespace,
        environmentName,
      });
    }
    return getFluxResourceStatus({
      url,
      watchPath,
      query: fluxKustomizationStatusQuery,
      variables: { configuration, namespace, environmentName, fluxResourcePath },
      field: kustomizationField,
      client,
    });
  },
  fluxHelmReleaseStatus(
    _,
    { configuration, namespace, environmentName, fluxResourcePath },
    { client },
  ) {
    const watchPath = buildFluxResourceWatchPath({
      namespace,
      apiVersion: helmReleasesApiVersion,
      resourceType: HELM_RELEASES_RESOURCE_TYPE,
    });
    let url;

    if (fluxResourcePath) {
      url = `${configuration.basePath}/apis/${fluxResourcePath}`;
    } else {
      url = buildFluxResourceUrl({
        basePath: configuration.basePath,
        resourceType: HELM_RELEASES_RESOURCE_TYPE,
        apiVersion: helmReleasesApiVersion,
        namespace,
        environmentName,
      });
    }
    return getFluxResourceStatus({
      url,
      watchPath,
      query: fluxHelmReleaseStatusQuery,
      variables: { configuration, namespace, environmentName, fluxResourcePath },
      field: helmReleaseField,
      client,
    });
  },
  fluxKustomizations(_, { configuration, namespace }) {
    const url = buildFluxResourceUrl({
      basePath: configuration.basePath,
      resourceType: KUSTOMIZATIONS_RESOURCE_TYPE,
      apiVersion: kustomizationsApiVersion,
      namespace,
    });
    return getFluxResources(configuration, url);
  },
  fluxHelmReleases(_, { configuration, namespace }) {
    const url = buildFluxResourceUrl({
      basePath: configuration.basePath,
      resourceType: HELM_RELEASES_RESOURCE_TYPE,
      apiVersion: helmReleasesApiVersion,
      namespace,
    });
    return getFluxResources(configuration, url);
  },
};
