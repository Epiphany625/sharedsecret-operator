kubectl create namespace staging
kubectl create namespace production

# CRD registration
kubectl apply -f config/crd/apps.leo.dev_sharedsecrets.yaml

kubectl create secret generic app-secret --from-literal=hello=world
kubectl apply -f config/samples/sharedsecrets.yaml

# now confirm the secrets are created in the corresponding namespace
sleep 2
kubectl -n staging get secrets
kubectl -n production get secrets

# now, delete the shared secrets, and confirm corresponding secret resources also got deleted. 
kubectl delete sharedsecret shared-app-secret
sleep 2
kubectl -n staging get secrets
kubectl -n production get secrets


kubectl apply -f config/samples/sharedsecrets.yaml

# make edits: add a new secret field. 
kubectl -n staging edit secrets app-secret

# wait for a while and confirm the edits are gone, because of the operators sync
sleep 2
kubectl -n staging get secrets

# clean up
kubectl delete sharedsecret shared-app-secret
kubectl delete crd sharedsecret
kubectl delete namespace staging
kubectl delete namespace production
kubectl delete secret app-secret
