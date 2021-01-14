# Migrating to version 2.0.x

As of the new release, the Custom Resource managed by the Mattermsot Operator changes from `ClusterInstallation` to `Mattermost`.
Besides the name change, some new functionality is introduced while other is removed.

`BlueGreen` and `Canary` deployments were not widely used and introduced a lot of complexity, so those features were dropped. In most cases, the multi-replica Mattermost cluster proved to be enough for save updates between versions.
Behavior similar to `BlueGreen` could be mimicked by using multiple Mattermost Instances.

## Automatic migration
It is possible for the Operator to migrate `ClusterInstallation` to `Mattermost` as long as it does not contain any of the unsupported features (like `BlueGreen` or `Canary`).
For migration to be possible, the Mattermost Operator needs to be in version `v1.12.x`.

> **NOTE:** Make sure that Mattermost Operator is in version `v1.12.x` before starting the migration.
> To do that, inspect the Operator image tag. To get full image, run:
> ```
> kubectl -n [OPERATOR_NAMESPACE] get deployment mattermost-operator -o jsonpath='{.spec.template.spec.containers[0].image}'
> ```
> If the Operator's minor version is earlier than `12`, update it to `v1.12.0` by applying manifests from LINK TO MANIFEST AFTER RELEASE. 

To run the migration, follow the steps:

1. Prepare the namespace and name of the `ClusterInstallation` and export those as environment variables:
    ```
    export CI_NAMESPACE=[YOUR_NAMESPACE]
    export CI_NAME=[NAME_OF_YOUR_INSTALLATION]
    ```

1. Trigger the migration by setting `spec.migrate` to `true` on the `ClusterInstsallation` instance:
    ```bash
    kubectl -n ${CI_NAMESPACE} patch clusterinstallation ${CI_NAME} --type merge --patch "spec:
      migrate: true"
    ```

1. Wait for migration to finish. The migration is done when the `ClusterInstallation` is removed and the `Mattermost` CR with the same name is created and it's `status.state` is equal to `stable`.

3. Ensure that `Mattermost` CR spec is correct and matches your needs:
    ```
    kubectl -n ${CI_NAMESPACE} get mm ${CI_NAME} -oyaml
    ```


### Troubleshooting the migration

To see the status and potential errors that occurred during the migration, query `status.migration` from the `ClusterInstallation`:
```
kubectl -n ${CI_NAMESPACE} get clusterinstallation ${CI_NAME} -o jsonpath='{.status.migration}'
```

If the migration failed, it can be reverted with several steps, which may vary depending on when the failure occurred.
> **CAUTION:** If the migration finished successfully it cannot be reverted.

1. Set `spec.migrate` to `false` or remove the field from the resource.
    ```bash
    kubectl -n ${CI_NAMESPACE} patch clusterinstallation ${CI_NAME} --type merge --patch "spec:
      migrate: false"
    ```

2. Remove the new `Mattermost` resource if it was created.
    ```bash
    kubectl -n ${CI_NAMESPACE} delete mm ${CI_NAME}
    ```

3. Remove new Deployment if it was created.
    ```bash
    kubectl -n ${CI_NAMESPACE} delete deployment ${CI_NAME}
    ```
