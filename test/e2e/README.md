# E2E Test

- Step 1: Run Searchlight operator with Icinga.

      $ kubectl create ns e2e-demo
      $ ./hack/deploy/searchlight.sh --namespace=e2e-demo --docker-registry=aerokite --enable-validating-webhook=true --rbac=true

- Step 2: Run E2E Test

      $ ./hack/make.py test e2e --icinga-reference=searchlight-operator@e2e-demo --provided-controller=true



## Delete 

    $ ./hack/deploy/searchlight.sh --namespace=e2e-demo --uninstall --purge
    $ kubectl delete ns e2e-demo