package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/swaggo/gin-swagger/swaggerFiles"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"

	"helm-proxy/docs"
)

type HelmConfig struct {
	UploadPath   string        `yaml:"uploadPath"`   //chart的上传路径
	TemplatePath string        `yaml:"templatePath"` //chart的模板路径
	SnapPath     string        `yaml:"snapPath"`     //上传chart库前的临时路径
	HelmRepos    []*repo.Entry `yaml:"helmRepos"`
}

var (
	settings            = cli.New()
	defaultUploadPath   = "./charts/upload"
	defaultTemplatePath = "./charts/template"
	defaultSnapPath     = "./charts/snap"
	helmConfig          = &HelmConfig{}
)

// 跨域
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("origin")
		if len(origin) == 0 {
			origin = c.Request.Header.Get("Origin")
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// @title Helm API Proxy
// @version 0.0.1
// @description This is a api proxy of helm.
// @contact.name polya
// @contact.email mika055@163.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
func main() {
	var (
		listenHost string
		listenPort string
		config     string
	)

	flag.Set("logtostderr", "true")
	pflag.CommandLine.StringVar(&listenHost, "addr", "127.0.0.1", "server listen addr")
	pflag.CommandLine.StringVar(&listenPort, "port", "18080", "server listen port")
	pflag.CommandLine.StringVar(&config, "config", "config.yaml", "helm proxy config")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	settings.AddFlags(pflag.CommandLine)
	pflag.Parse()
	defer glog.Flush()

	configBody, err := ioutil.ReadFile(config)
	if err != nil {
		glog.Fatalln(err)
	}
	err = yaml.Unmarshal(configBody, helmConfig)
	if err != nil {
		glog.Fatalln(err)
	}

	// upload chart path
	if helmConfig.UploadPath == "" {
		helmConfig.UploadPath = defaultUploadPath
	} else {
		if !filepath.IsAbs(helmConfig.UploadPath) {
			glog.Fatalln("charts upload path is not absolute")
		}
	}
	// chart template path
	if helmConfig.TemplatePath == "" {
		helmConfig.TemplatePath = defaultTemplatePath
	} else {
		if !filepath.IsAbs(helmConfig.TemplatePath) {
			glog.Fatalln("charts template path is not absolute")
		}
	}
	// chart snap path
	if helmConfig.SnapPath == "" {
		helmConfig.SnapPath = defaultSnapPath
	} else {
		if !filepath.IsAbs(helmConfig.SnapPath) {
			glog.Fatalln("charts snap path is not absolute")
		}
	}
	_, err = os.Stat(helmConfig.UploadPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(helmConfig.UploadPath, 0755)
			if err != nil {
				glog.Fatalln(err)
			}
		} else {
			glog.Fatalln(err)
		}
	}

	// init repo
	for _, c := range helmConfig.HelmRepos {
		err = initRepository(c)
		if err != nil {
			glog.Fatalln(err)
		}
	}

	// router
	router := gin.Default()
	router.Use(cors()) //跨域设置
	router.Use(gin.Recovery())
	router.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "Welcome helm proxy server")
	})

	// swago定义
	docs.SwaggerInfo.Host = listenHost + ":" + listenPort
	docs.SwaggerInfo.BasePath = "/api/"
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/swagger/doc.json")))

	// register router
	RegisterRouter(router)

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", listenHost, listenPort),
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			glog.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	glog.Infoln("Shutdown Server ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
