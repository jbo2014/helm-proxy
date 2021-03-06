package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	cm "github.com/chartmuseum/helm-push/pkg/chartmuseum"
	"github.com/chartmuseum/helm-push/pkg/helm"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/repo"
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
	all := &ChartView{}
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
		if chrt.Files == nil {
			respOK(c, "")
		} else {
			respOK(c, string(findReadme(chrt.Files).Data))
		}
		return
	} else if client.OutputFormat == action.ShowAll {
		if chrt.Files == nil {
			all.Readme = ""
		} else {
			all.Readme = string(findReadme(chrt.Files).Data)
		}
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

type ChartView struct {
	Chart    chart.Metadata         `json:"chart"`
	Values   map[string]interface{} `json:"values"`
	Readme   string                 `json:"readme"`
	Template []*file                `json:"template"`
}

// @Summary			显示chart解析后的k8s部署yaml
// @Description 	显示chart的k8s部署yaml，如果多个文件则合并到一个yaml一起展示出来
// @Tags			Chart
// @Param 			chart query string true "chart名称"
// @Param 			values body map[string]interface{} false "变量"
// @Success 		200 {object} respBody
// @Router 			/charts/template [post]
func showTemplate(c *gin.Context) {
	chart := c.Query("chart")
	// var showFiles []string
	// var compose string

	var vals map[string]interface{}
	err := c.ShouldBindJSON(&vals)
	if err != nil {
		respErr(c, err)
		return
	}

	actionConfig, err := actionConfigInit(settings.Namespace())
	client := action.NewInstall(actionConfig)
	client.DryRun = true
	client.ReleaseName = "RELEASE-NAME"
	client.Replace = true // Skip the name check
	// client.ClientOnly = !validate
	// client.APIVersions = chartutil.VersionSet(extraAPIs)
	// client.IncludeCRDs = includeCrds

	rel, err := runInstall(chart, client, vals)
	if err != nil {
		respErr(c, err)
	}
	respOK(c, rel.Manifest)
}

func writeToFile(outputDir string, name string, data string, append bool) error {
	outfileName := strings.Join([]string{outputDir, name}, string(filepath.Separator))

	err := ensureDirectoryForFile(outfileName)
	if err != nil {
		return err
	}

	f, err := createOrOpenFile(outfileName, append)
	if err != nil {
		return err
	}

	defer f.Close()

	_, err = f.WriteString(fmt.Sprintf("---\n# Source: %s\n%s\n", name, data))

	if err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", outfileName)
	return nil
}

func createOrOpenFile(filename string, append bool) (*os.File, error) {
	if append {
		return os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0600)
	}
	return os.Create(filename)
}

func ensureDirectoryForFile(file string) error {
	baseDir := path.Dir(file)
	_, err := os.Stat(baseDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return os.MkdirAll(baseDir, 0755)
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
// @Param 			newChart body chartNew true "chart信息"
// @Success 		200 {object} respBody
// @Router 			/charts/create [post]
func createChart(c *gin.Context) {
	var chartObj chartNew
	if err := c.BindJSON(&chartObj); err != nil {
		respErr(c, err)
		return
	}

	path, err := ioutil.TempDir(helmConfig.SnapPath, chartObj.Chart.Name+"."+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err == nil {
		//创建Chart.yaml
		if f, err := os.Create(path + "/Chart.yaml"); err == nil {
			data, _ := yaml.Marshal(chartObj.Chart)
			f.Write(data)
		}
		//创建values.yaml
		if f, err := os.Create(path + "/values.yaml"); err == nil {
			data, _ := yaml.Marshal(chartObj.Values)
			f.Write(data)
		}
		//创建README.md
		if f, err := os.Create(path + "/README.md"); err == nil {
			f.WriteString(chartObj.Readme)
		}
		if err := os.MkdirAll(path+"/templates", 0755); err == nil {
			//创建k8s-compose.yaml
			if f, err := os.Create(path + "/templates/k8s-compose.yaml"); err == nil {
				f.WriteString(chartObj.Template[0].Data)
			}
		}
	}
	defer os.RemoveAll(path) //销毁临时模板文件夹

	// 确定repo对象
	repo := &repo.Entry{}
	for _, r := range helmConfig.HelmRepos {
		if r.Name == chartObj.RepoName {
			repo = r
			break
		}
	}

	pusher := &pusher{
		chartName:    path,
		chartVersion: chartObj.Chart.Version,
		repoName:     chartObj.RepoName,
	}

	if err := push(pusher, repo); err != nil {
		respErr(c, err)
	} else {
		respOK(c, "ok")
	}
	return
}

type chartNew struct {
	RepoName string `json:"repoName"`
	*ChartView
}

//
func updateChart(c *gin.Context) {

}

// region:上传chart库共通

type pusher struct {
	chartName    string
	chartVersion string
	repoName     string
	accessToken  string
	authHeader   string
	contextPath  string
	forceUpload  bool
	useHTTP      bool
}

func push(p *pusher, r *repo.Entry) error {
	var err error

	// If the argument looks like a URL, just create a temp repo object
	// instead of looking for the entry in the local repository list
	if regexp.MustCompile(`^https?://`).MatchString(p.repoName) {
		//r, err = helm.TempRepoFromURL(p.repoName)
		p.repoName = r.URL
	} else {
		//repo, err = helm.GetRepoByName(p.repoName)
	}

	if err != nil {
		return err
	}

	chart, err := helm.GetChartByName(p.chartName)
	if err != nil {
		return err
	}

	// version override
	if p.chartVersion != "" {
		chart.SetVersion(p.chartVersion)
	}

	// username/password override(s)
	username := r.Username
	password := r.Password

	// in case the repo is stored with cm:// protocol, remove it
	var url string
	if p.useHTTP {
		url = strings.Replace(r.URL, "cm://", "http://", 1)
	} else {
		url = strings.Replace(r.URL, "cm://", "https://", 1)
	}

	client, err := cm.NewClient(
		cm.URL(url),
		cm.Username(username),
		cm.Password(password),
		cm.AccessToken(p.accessToken),
		cm.AuthHeader(p.authHeader),
		cm.ContextPath(p.contextPath),
		cm.CAFile(r.CAFile),
		cm.CertFile(r.CertFile),
		cm.KeyFile(r.KeyFile),
		cm.InsecureSkipVerify(r.InsecureSkipTLSverify),
	)

	if err != nil {
		return err
	}

	// update context path if not overrided
	// if p.contextPath == "" {
	// 	index, err := helm.GetIndexByRepo(nil, getIndexDownloader(client))
	// 	if err != nil {
	// 		return err
	// 	}
	// 	client.Option(cm.ContextPath(index.ServerInfo.ContextPath))
	// }

	tmp, err := ioutil.TempDir("", "helm-push-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	chartPackagePath, err := helm.CreateChartPackage(chart, tmp)
	if err != nil {
		return err
	}

	fmt.Printf("Pushing %s to %s...\n", filepath.Base(chartPackagePath), p.repoName)
	resp, err := client.UploadChartPackage(chartPackagePath, p.forceUpload)
	if err != nil {
		return err
	}

	return handlePushResponse(resp)
}

func getIndexDownloader(client *cm.Client) helm.IndexDownloader {
	return func() ([]byte, error) {
		resp, err := client.DownloadFile("index.yaml")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, getChartmuseumError(b, resp.StatusCode)
		}
		return b, nil
	}
}

func handlePushResponse(resp *http.Response) error {
	if resp.StatusCode != 201 {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return getChartmuseumError(b, resp.StatusCode)
	}
	fmt.Println("Done.")
	return nil
}

func getChartmuseumError(b []byte, code int) error {
	var er struct {
		Error string `json:"error"`
	}
	err := json.Unmarshal(b, &er)
	if err != nil || er.Error == "" {
		return fmt.Errorf("%d: could not properly parse response JSON: %s", code, string(b))
	}
	return fmt.Errorf("%d: %s", code, er.Error)
}

// endregion
