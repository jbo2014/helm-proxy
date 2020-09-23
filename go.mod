module helm-proxy

go 1.14

require (
	github.com/Masterminds/semver v1.5.0
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751
	github.com/chartmuseum/helm-push v0.7.1
	github.com/gin-gonic/gin v1.6.3
	github.com/gofrs/flock v0.7.1
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	github.com/swaggo/gin-swagger v1.2.0
	github.com/swaggo/swag v1.6.7
	helm.sh/helm/v3 v3.3.0
	k8s.io/helm v2.16.12+incompatible // indirect
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/yaml v1.2.0
)
