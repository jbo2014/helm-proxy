package main

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
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
// @Description 	根据chart名称，获取chart的readme、values、chart信息
// @Tags			Chart
// @Param 			chart query string true "chart名称"
// @Param   		version query string false "chart版本"
// @Param   		info query string false "Enums(all, readme, values, chart)"
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
	if info == string(action.ShowAll) {
		client.OutputFormat = action.ShowAll
	} else if info == string(action.ShowChart) {
		client.OutputFormat = action.ShowChart
	} else if info == string(action.ShowReadme) {
		client.OutputFormat = action.ShowReadme
	} else if info == string(action.ShowValues) {
		client.OutputFormat = action.ShowValues
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

	if client.OutputFormat == action.ShowChart {
		respOK(c, chrt.Metadata)
		return
	}
	if client.OutputFormat == action.ShowValues {
		values := make([]*file, 0, len(chrt.Raw))
		for _, v := range chrt.Raw {
			values = append(values, &file{
				Name: v.Name,
				Data: string(v.Data),
			})
		}
		respOK(c, values)
		return
	}
	if client.OutputFormat == action.ShowReadme {
		respOK(c, string(findReadme(chrt.Files).Data))
		return
	}
}

// @Summary			显示chart的部署yaml
// @Description 	显示chart的k8s部署yaml，如果多个文件则合并到一个yaml一起展示出来
// @Tags			Chart
// @Param 			chart query string true "chart名称"
// @Param 			raw query string false "是否不解析模板，1.不解析；0.解析为部署文件"
// @Success 		200 {object} respBody
// @Router 			/charts/template [post]
func showTemplate(c *gin.Context) {
	raw := c.DefaultQuery("raw", "0")
	if raw == "0" {
		// 显示编译变量后的yaml文件
	} else if raw == "1" {
		// 显示编译变量前的yaml模板
	}
	respOK(c, "haha")
	return
}

// @Summary			下载chart
// @Description 	helm pull
// @Tags			Chart
// @Param 			chart query string true "chart URL | repo/chartname"
// @Success 		200 {object} respBody
// @Router 			/charts/export [post]
func exportChart(c *gin.Context) {
	// params := c.Query()

	// filePath := c.Query("url")
	// //打开文件
	// fileTmp, errByOpenFile := os.Open(filePath)
	// defer fileTmp.Close()

	// //获取文件的名称
	// fileName := path.Base(filePath)
	// c.Header("Content-Type", "application/octet-stream")
	// c.Header("Content-Disposition", "attachment; filename="+fileName)
	// c.Header("Content-Transfer-Encoding", "binary")
	// c.Header("Cache-Control", "no-cache")
	// if len(filePath) == 0 || len(fileName) == 0 || errByOpenFile != nil {
	// 	respErr(c, errors.Errorf("no exist"))
	// 	c.Redirect(http.StatusFound, "/404")
	// 	return
	// }
	// c.Header("Content-Type", "application/octet-stream")
	// c.Header("Content-Disposition", "attachment; filename="+fileName)
	// c.Header("Content-Transfer-Encoding", "binary")

	// c.File(filePath)
	// return
}

func pullChart(c *gin.Context) {
	// p := c.Query("chart")
	// if len(p) == 0 {
	// 	respErr(c, errors.Errorf("need \"chart URL\" or \"repo/chartname\""))
	// 	return
	// }

	// if p.Verify {
	// 	c.Verify = downloader.VerifyAlways
	// } else if p.VerifyLater {
	// 	c.Verify = downloader.VerifyLater
	// }

	// // If untar is set, we fetch to a tempdir, then untar and copy after
	// // verification.
	// dest := p.DestDir
	// if p.Untar {
	// 	var err error
	// 	dest, err = ioutil.TempDir("", "helm-")
	// 	if err != nil {
	// 		return out.String(), errors.Wrap(err, "failed to untar")
	// 	}
	// 	defer os.RemoveAll(dest)
	// }

	// if p.RepoURL != "" {
	// 	chartURL, err := repo.FindChartInAuthRepoURL(p.RepoURL, p.Username, p.Password, chartRef, p.Version, p.CertFile, p.KeyFile, p.CaFile, getter.All(p.Settings))
	// 	if err != nil {
	// 		return out.String(), err
	// 	}
	// 	chartRef = chartURL
	// }

	// saved, v, err := c.DownloadTo(chartRef, p.Version, dest)
	// if err != nil {
	// 	return out.String(), err
	// }

	// if p.Verify {
	// 	for name := range v.SignedBy.Identities {
	// 		fmt.Fprintf(&out, "Signed by: %v\n", name)
	// 	}
	// 	fmt.Fprintf(&out, "Using Key With Fingerprint: %X\n", v.SignedBy.PrimaryKey.Fingerprint)
	// 	fmt.Fprintf(&out, "Chart Hash Verified: %s\n", v.FileHash)
	// }

	// // After verification, untar the chart into the requested directory.
	// if p.Untar {
	// 	ud := p.UntarDir
	// 	if !filepath.IsAbs(ud) {
	// 		ud = filepath.Join(p.DestDir, ud)
	// 	}
	// 	// Let udCheck to check conflict file/dir without replacing ud when untarDir is the current directory(.).
	// 	udCheck := ud
	// 	if udCheck == "." {
	// 		_, udCheck = filepath.Split(chartRef)
	// 	} else {
	// 		_, chartName := filepath.Split(chartRef)
	// 		udCheck = filepath.Join(udCheck, chartName)
	// 	}
	// 	if _, err := os.Stat(udCheck); err != nil {
	// 		if err := os.MkdirAll(udCheck, 0755); err != nil {
	// 			return out.String(), errors.Wrap(err, "failed to untar (mkdir)")
	// 		}

	// 	} else {
	// 		return out.String(), errors.Errorf("failed to untar: a file or directory with the name %s already exists", udCheck)
	// 	}

	// 	return out.String(), chartutil.ExpandFile(ud, saved)
	// }
	// return out.String(), nil
}
