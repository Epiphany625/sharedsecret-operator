kubectl create namespace staging
kubectl create namespace production

# CRD registration
kubectl apply -f config/crd/apps.leo.dev_sharedsecrets.yaml

kubectl create secret generic app-secret --from-literal=hello=world
kubectl apply -f config/samples/sharedsecrets.yaml

