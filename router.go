package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"

	_ "helm-proxy/docs"
)

type respBody struct {
	Code  int         `json:"code"` // 0 or 1, 0 is ok, 1 is error
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func respErr(c *gin.Context, err error) {
	glog.Warningln(err)

	c.JSON(http.StatusOK, &respBody{
		Code:  1,
		Error: err.Error(),
	})
}

func respOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, &respBody{
		Code: 0,
		Data: data,
	})
}

func RegisterRouter(router *gin.Engine) {
	// helm env
	envs := router.Group("/api/envs")
	{
		envs.GET("", getHelmEnvs)
	}

	// helm repo
	repositories := router.Group("/api/repos")
	{
		// helm search repo
		repositories.GET("/charts", listRepoCharts)
		// helm repo list
		repositories.GET("", listRepositories)
		// helm repo add [name] [url] [flags]
		repositories.POST("/add", addRepository)
		// helm repo remove [name1 name2]
		repositories.DELETE("/remove/:reponame", removeRepository)
		// helm repo update
		repositories.PUT("/update", updateRepositories)
	}

	// helm chart
	charts := router.Group("/api/charts")
	{
		// helm show all/readme/values/chart
		charts.GET("", showChart)
		// helm template
		charts.POST("/template", showTemplate)
		// helm pull
		charts.POST("/export", exportChart)
		// upload chart
		charts.POST("/upload", uploadChart)
		// list uploaded charts
		charts.GET("/upload", listUploadedCharts)
	}

	// helm release
	releases := router.Group("/api/namespaces/:namespace/releases")
	{
		// helm list releases ->  helm list
		releases.GET("", listReleases)
		// helm get
		releases.GET("/:release", showReleaseInfo)
		// helm install
		releases.POST("/:release", installRelease)
		// helm upgrade
		releases.PUT("/:release", upgradeRelease)
		// helm uninstall
		releases.DELETE("/:release", uninstallRelease)
		// helm rollback
		releases.PUT("/:release/versions/:reversion", rollbackRelease)
		// helm status <RELEASE_NAME>
		releases.GET("/:release/status", getReleaseStatus)
		// helm release history
		releases.GET("/:release/histories", listReleaseHistories)
	}
}
