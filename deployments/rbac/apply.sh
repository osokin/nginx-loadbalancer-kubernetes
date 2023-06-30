#!/bin/bash

pushd "$(dirname "$0")"

echo "Applying all RBAC resources..."

kubectl apply -f serviceaccount.yaml
kubectl apply -f clusterrole.yaml
kubectl apply -f clusterrolebinding.yaml
kubectl apply -f secret.yaml

popd
