package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

var readmeFileNames = []string{"readme.md", "readme.txt", "readme"}

type file struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func findReadme(files []*chart.File) (file *chart.File) {
	for _, file := range files {
		for _, n := range readmeFileNames {
			if strings.EqualFold(file.Name, n) {
				return file
			}
		}
	}
	return nil
}

// @Summary 		获取chart详细信息
// @Description 	根据chart名称，获取chart的readme、values、chart、template信息
// @Tags			Chart
// @Param 			chart query string true "chart名称"
// @Param   		version query string false "chart版本"
// @Param   		info query string false "Enums(all, readme, values, chart、template)"
// @Success 		200 {object} respBody
// @Router 			/charts [get]
func showChart(c *gin.Context) {
	name := c.Query("chart")
	if name == "" {
		respErr(c, fmt.Errorf("chart name can not be empty"))
		return
	}
	// local charts with abs path *.tgz
	splitChart := strings.Split(name, ".")
	if splitChart[len(splitChart)-1] == "tgz" {
		name = helmConfig.UploadPath + "/" + name
	}

	info := c.DefaultQuery("info", "all") // readme, values, chart
	version := c.Query("version")

	client := action.NewShow(action.ShowAll)
	client.Version = version
	all := &showAll{}
	if info == string(action.ShowAll) {
		client.OutputFormat = action.ShowAll
	} else if info == string(action.ShowChart) {
		client.OutputFormat = action.ShowChart
	} else if info == string(action.ShowReadme) {
		client.OutputFormat = action.ShowReadme
	} else if info == string(action.ShowValues) {
		client.OutputFormat = action.ShowValues
	} else if strings.EqualFold(info, "temp") {
		client.OutputFormat = "template"
	} else {
		respErr(c, fmt.Errorf("bad info %s, chart info only support readme/values/chart", info))
		return
	}

	cp, err := client.ChartPathOptions.LocateChart(name, settings)
	if err != nil {
		respErr(c, err)
		return
	}

	chrt, err := loader.Load(cp)
	if err != nil {
		respErr(c, err)
		return
	}
	// 整理chart的chart信息
	if client.OutputFormat == action.ShowChart {
		respOK(c, chrt.Metadata)
		return
	} else if client.OutputFormat == action.ShowAll {
		all.Chart = *chrt.Metadata
	}
	// 整理chart的values
	if client.OutputFormat == action.ShowValues || client.OutputFormat == action.ShowAll {
		values := map[string]interface{}{}

		for _, f := range chrt.Raw {
			if f.Name == chartutil.ValuesfileName {
				err = yaml.Unmarshal(f.Data, &values)
				break
			}
		}
		if client.OutputFormat == action.ShowValues {
			respOK(c, values)
			return
		} else if client.OutputFormat == action.ShowAll {
			all.Values = values
		}
	}
	// 整理chart的readme
	if client.OutputFormat == action.ShowReadme {
		respOK(c, string(findReadme(chrt.Files).Data))
		return
	} else if client.OutputFormat == action.ShowAll {
		all.Readme = string(findReadme(chrt.Files).Data)
	}
	// 整理chart的template
	if client.OutputFormat == "template" || client.OutputFormat == action.ShowAll {
		values := make([]*file, 0, len(chrt.Raw))
		bol := false
		for _, v := range chrt.Raw {
			if bol, _ = regexp.MatchString(`templates/(.*).(yaml|yml)`, v.Name); bol {
				values = append(values, &file{
					Name: v.Name,
					Data: string(v.Data),
				})
			}
		}
		if client.OutputFormat == "template" {
			respOK(c, values)
			return
		} else if client.OutputFormat == action.ShowAll {
			all.Template = values
		}
	}

	//返回all
	respOK(c, all)
	return
}

type showAll struct {
	Chart    chart.Metadata         `json:"chart"`
	Values   map[string]interface{} `json:"values"`
	Readme   string                 `json:"readme"`
	Template []*file                `json:"template"`
}

// @Summary			显示chart的部署yaml
// @Description 	显示chart的k8s部署yaml，如果多个文件则合并到一个yaml一起展示出来
// @Tags			Chart
// @Param 			chart query string true "chart名称"
// @Success 		200 {object} respBody
// @Router 			/charts/template [post]
func showTemplate(c *gin.Context) {
	// name := c.Query("chart")
	// respOK(c, "haha")
	// return
}

// @Summary			下载chart
// @Description 	helm pull
// @Tags			Chart
// @Param 			url body string true "chart的url"
// @Success 		200 {object} respBody
// @Router 			/charts/export [post]
func exportChart(c *gin.Context) {
	url := c.Query("url")
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename="+url)
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Cache-Control", "no-cache")
	if len(url) == 0 {
		respErr(c, errors.Errorf("no exist"))
		return
	}
	c.File(url)
	return
}
