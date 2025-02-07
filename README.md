# aurora-controller

A set of controllers that help to further configure the Aurora platform.

## Image Pull Secrets

To add an imagePullSecret to all service accounts:

```sh
./aurora-controller image-pull-secrets --image-pull-secret=artifactory --kubeconfig path/to/kubeconfig
```
