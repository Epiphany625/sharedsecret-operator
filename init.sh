# handle go dependencies
V=v0.37.0
go get k8s.io/api@$V k8s.io/apimachinery@$V k8s.io/client-go@$V k8s.io/code-generator@$V

# generate folder structure. 
CRD="sharedsecret"
go mod init github.com/${CRD}-operator 
mkdir -p hack pkg/apis/${CRD}/v1alpha1 config/crd config/samples cmd/operator controller


controller-gen object:headerFile=hack/boilerplate.go.txt paths=./pkg/apis/...
controller-gen crd paths=./pkg/apis/... output:crd:dir=config/crd

