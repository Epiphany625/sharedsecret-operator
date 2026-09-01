# sharedsecret-operator

A Kubernetes operator.

TODO list:

1. Add the status update functionality, so that status can be updated successfully.
2. Come up with a way to use namespace informer to automatically add a secret when certain namespace got created, instead of waiting for full informer cache sync.
3. Write unit & integration tests for using fake, envtest, etc.
4. Add Prometheus & Grafana metrics support for this operator. Make it configurable through main.go
5. Try to make sharedsecret controller work within a cluster, and test out its behavior.
6. Finish project.
