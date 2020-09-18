package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
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
	all := &chartView{}
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

type chartView struct {
	Chart    chart.Metadata         `json:"chart"`
	Values   map[string]interface{} `json:"values"`
	Readme   string                 `json:"readme"`
	Template []*file                `json:"template"`
}

// @Summary			显示chart解析后的k8s部署yaml
// @Description 	显示chart的k8s部署yaml，如果多个文件则合并到一个yaml一起展示出来
// @Tags			Chart
// @Param 			chart query string true "chart名称"
// @Success 		200 {object} respBody
// @Router 			/charts/template [post]
func showTemplate(c *gin.Context) {
	// rel, err := runInstall(args, client, valueOpts, out)
	// if err != nil && !settings.Debug {
	// 	if rel != nil {
	// 		respErr(c, errors.Errorf("%w\n\nUse --debug flag to render out invalid YAML", err))
	// 		return
	// 	}
	// 	respErr(c, errors.Errorf(err))
	// 	return
	// }

	// // We ignore a potential error here because, when the --debug flag was specified,
	// // we always want to print the YAML, even if it is not valid. The error is still returned afterwards.
	// if rel != nil {
	// 	var manifests bytes.Buffer
	// 	fmt.Fprintln(&manifests, strings.TrimSpace(rel.Manifest))
	// 	if !client.DisableHooks {
	// 		fileWritten := make(map[string]bool)
	// 		for _, m := range rel.Hooks {
	// 			if client.OutputDir == "" {
	// 				fmt.Fprintf(&manifests, "---\n# Source: %s\n%s\n", m.Path, m.Manifest)
	// 			} else {
	// 				newDir := client.OutputDir
	// 				if client.UseReleaseName {
	// 					newDir = filepath.Join(client.OutputDir, client.ReleaseName)
	// 				}
	// 				err = writeToFile(newDir, m.Path, m.Manifest, fileWritten[m.Path])
	// 				if err != nil {
	// 					return err
	// 				}
	// 				fileWritten[m.Path] = true
	// 			}

	// 		}
	// 	}

	// 	// if we have a list of files to render, then check that each of the
	// 	// provided files exists in the chart.
	// 	if len(showFiles) > 0 {
	// 		// This is necessary to ensure consistent manifest ordering when using --show-only
	// 		// with globs or directory names.
	// 		splitManifests := releaseutil.SplitManifests(manifests.String())
	// 		manifestsKeys := make([]string, 0, len(splitManifests))
	// 		for k := range splitManifests {
	// 			manifestsKeys = append(manifestsKeys, k)
	// 		}
	// 		sort.Sort(releaseutil.BySplitManifestsOrder(manifestsKeys))

	// 		manifestNameRegex := regexp.MustCompile("# Source: [^/]+/(.+)")
	// 		var manifestsToRender []string
	// 		for _, f := range showFiles {
	// 			missing := true
	// 			// Use linux-style filepath separators to unify user's input path
	// 			f = filepath.ToSlash(f)
	// 			for _, manifestKey := range manifestsKeys {
	// 				manifest := splitManifests[manifestKey]
	// 				submatch := manifestNameRegex.FindStringSubmatch(manifest)
	// 				if len(submatch) == 0 {
	// 					continue
	// 				}
	// 				manifestName := submatch[1]
	// 				// manifest.Name is rendered using linux-style filepath separators on Windows as
	// 				// well as macOS/linux.
	// 				manifestPathSplit := strings.Split(manifestName, "/")
	// 				// manifest.Path is connected using linux-style filepath separators on Windows as
	// 				// well as macOS/linux
	// 				manifestPath := strings.Join(manifestPathSplit, "/")

	// 				// if the filepath provided matches a manifest path in the
	// 				// chart, render that manifest
	// 				if matched, _ := filepath.Match(f, manifestPath); !matched {
	// 					continue
	// 				}
	// 				manifestsToRender = append(manifestsToRender, manifest)
	// 				missing = false
	// 			}
	// 			if missing {
	// 				return fmt.Errorf("could not find template %s in chart", f)
	// 			}
	// 		}
	// 		for _, m := range manifestsToRender {
	// 			fmt.Fprintf(out, "---\n%s\n", m)
	// 		}
	// 	} else {
	// 		fmt.Fprintf(out, "%s", manifests.String())
	// 	}
	// }

	// respErr(c, errors.Errorf(err))
	// return
}

// @Summary			获取chart下载地址
// @Description 	获取chart的下载地址
// @Tags			Chart
// @Param 			factor body downFactor true "带下载的chart路径信息"
// @Success 		200 {object} respBody
// @Router 			/charts/export [post]
func exportChart(c *gin.Context) {
	//arg := c.Query("param")
	// c.Header("Content-Type", "application/x-gtar")
	// c.Header("Content-Disposition", "attachment; filename="+url)
	// c.Header("Content-Transfer-Encoding", "binary")
	// c.Header("Cache-Control", "no-cache")
	// if len(url) == 0 {
	// 	respErr(c, errors.Errorf("no exist"))
	// 	return
	// }
	// c.File(url)
	var factor downFactor
	if c.Bind(&factor) != nil {
		respErr(c, errors.Errorf("missing parameters"))
		return
	}
	bol, _ := regexp.MatchString(`^([hH][tT]{2}[pP]:\/\/|[hH][tT]{2}[pP][sS]:\/\/|www\.)(([A-Za-z0-9-~]+)\.)+([A-Za-z0-9-~\.\/])+(.tgz)$`, factor.ChartURL)
	if bol {
		// 是正常的可下载路径
		respOK(c, factor.ChartURL)
	} else {
		// 非正常可下载路径
		// 可能只有chart文件名，缺少仓库地址，添加上后再试试
		respOK(c, factor.RepoURL+"/"+factor.ChartURL)
	}
	return
}

type downFactor struct {
	RepoURL  string `json:"repoUrl"`
	ChartURL string `json:"chartUrl"`
}

// @Summary			新建chart
// @Description 	新建一个chart并上传至镜像库
// @Tags			Chart
// @Param			repoUrl query string true "chart仓库的上传地址"
// @Param 			meta body chart.Metadata true "chart基本信息"
// @Param 			readme query string true "chart的readme"
// @Param 			values body map[string]interface{} true "chart的values"
// @Param 			compose query string true "chart的k8s部署yaml"
// @Success 		200 {object} respBody
// @Router 			/charts/create [post]
func createChart(c *gin.Context) {
	var meta chart.Metadata
	if err := c.BindJSON(&meta); err != nil {
		respErr(c, err)
		return
	}
	readme := c.Query("readme")
	var values map[string]interface{}
	if err := c.BindJSON(&values); err != nil {
		respErr(c, err)
		return
	}
	compose := c.Query("compose")

	// 创建chart目录和文件
	path := helmConfig.SnapPath + "/" + meta.Name
	os.MkdirAll(path, 0755) //创建chart目录
	//创建Chart.yaml
	if file, err := os.Create(path + "/Chart.yaml"); err == nil {
		data, _ := json.Marshal(meta)
		file.Write(data)
	}
	//创建values.yaml
	if file, err := os.Create(path + "/values.yaml"); err == nil {
		data, _ := json.Marshal(values)
		file.Write(data)
	}
	//创建readme.md
	if file, err := os.Create(path + "/readme.md"); err == nil {
		file.WriteString(readme)
	}
	os.MkdirAll(path+"/templates", 0755) //创建chart目录
	//创建k8s-compose.yaml
	if file, err := os.Create(path + "/templates/k8s-compose.yaml"); err == nil {
		file.WriteString(compose)
	}

	repo := c.Query("repoUrl")
	pushChart(path, repo)
}

//
func updateChart(c *gin.Context) {

}

func pushChart(url string, arg ...string) error {
	client := &http.Client{}
	req, err := http.NewRequest(
		"POST",
		url,
		strings.NewReader("name=abc"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
}
