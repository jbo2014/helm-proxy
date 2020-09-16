package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/flock"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/cmd/helm/search"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/helmpath"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

const searchMaxScore = 25

type repoChartElement struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Tags        string `json:"tags"`
}

type repoChartList []repoChartElement

func applyConstraint(version string, versions bool, res []*search.Result) ([]*search.Result, error) {
	if len(version) == 0 {
		return res, nil
	}

	constraint, err := semver.NewConstraint(version)
	if err != nil {
		return res, errors.Wrap(err, "an invalid version/constraint format")
	}

	data := res[:0]
	foundNames := map[string]bool{}
	for _, r := range res {
		if _, found := foundNames[r.Name]; found {
			continue
		}
		v, err := semver.NewVersion(r.Chart.Version)
		if err != nil || constraint.Check(v) {
			data = append(data, r)
			if !versions {
				foundNames[r.Name] = true // If user hasn't requested all versions, only show the latest that matches
			}
		}
	}

	return data, nil
}

func buildSearchIndex(version string) (*search.Index, error) {
	i := search.NewIndex()
	for _, re := range helmConfig.HelmRepos {
		n := re.Name
		f := filepath.Join(settings.RepositoryCache, helmpath.CacheIndexFile(n))
		ind, err := repo.LoadIndexFile(f)
		if err != nil {
			glog.Warningf("WARNING: Repo %q is corrupt or missing. Try 'helm repo update'.", n)
			continue
		}

		i.AddRepo(n, ind, len(version) > 0)
	}
	return i, nil
}

func initRepository(c *repo.Entry) error {
	// Ensure the file directory exists as it is required for file locking
	err := os.MkdirAll(filepath.Dir(settings.RepositoryConfig), os.ModePerm)
	if err != nil && !os.IsExist(err) {
		return err
	}

	// Acquire a file lock for process synchronization
	fileLock := flock.New(strings.Replace(settings.RepositoryConfig, filepath.Ext(settings.RepositoryConfig), ".lock", 1))
	lockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(lockCtx, time.Second)
	if err == nil && locked {
		defer fileLock.Unlock()
	}
	if err != nil {
		return err
	}

	b, err := ioutil.ReadFile(settings.RepositoryConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var f repo.File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return err
	}

	r, err := repo.NewChartRepository(c, getter.All(settings))
	if err != nil {
		return err
	}

	if _, err := r.DownloadIndexFile(); err != nil {
		return err
	}

	f.Update(c)

	if err := f.WriteFile(settings.RepositoryConfig, 0644); err != nil {
		return err
	}

	return nil
}

// @Summary 		查找chart/列出本地库中的所有chart
// @Description 	在本地库中查找chart，如没有keyword则列出所有chart
// @Tags			Repository
// @Param 			keyword query string false "搜索关键字"
// @Param   		version query string false "chart版本"
// @Param   		versions query bool false "如果true，查询出每个chart的所有版本；false，只列出每个chart最新版"
// @Success 		200 {object} respBody
// @Router 			/repos/charts [get]
func listRepoCharts(c *gin.Context) {
	version := c.Query("version")   // chart version
	versions := c.Query("versions") // if "true", all versions
	keyword := c.Query("keyword")   // search keyword

	// default stable
	if version == "" {
		version = ">0.0.0"
	}

	index, err := buildSearchIndex(version)
	if err != nil {
		respErr(c, err)
		return
	}

	var res []*search.Result
	if keyword == "" {
		res = index.All()
	} else {
		res, err = index.Search(keyword, searchMaxScore, false)
	}

	search.SortScore(res)
	var versionsB bool
	if versions == "true" {
		versionsB = true
	}
	data, err := applyConstraint(version, versionsB, res)
	if err != nil {
		respErr(c, err)
		return
	}
	chartList := make(repoChartList, 0, len(data))
	for _, v := range data {
		chartList = append(chartList, repoChartElement{
			Name:        v.Name,
			Version:     v.Chart.Version,
			AppVersion:  v.Chart.AppVersion,
			Description: v.Chart.Description,
			Icon:        v.Chart.Icon,
			Tags:        v.Chart.Tags,
		})
	}

	respOK(c, chartList)
}

type repositoryElement struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// @Summary 		获取所有本地库
// @Description 	列出所有repo
// @Tags			Repository
// @Success 		200 {object} respBody
// @Router 			/repos [get]
func listRepositories(c *gin.Context) {
	f, err := repo.LoadFile(settings.RepositoryConfig)
	if os.IsNotExist(err) || (len(f.Repositories) == 0) {
		respErr(c, err)
	}

	repoList := make([]repositoryElement, 0, len(f.Repositories))
	for _, re := range f.Repositories {
		repoList = append(repoList, repositoryElement{
			Name: re.Name,
			URL:  re.URL,
		})
	}

	respOK(c, repoList)
}

// @Summary			添加chart镜像库
// @Description 	通过名称，删除一个镜像库
// @Tags			Repository
// @Param           repoinfo body repoAddOptions true "仓库信息"
// @Success 		200 {object} respBody
// @Router 			/repos/add [post]
func addRepository(c *gin.Context) {
	var info repoAddOptions

	if c.Bind(&info) == nil {
	}

	o := info
	o.repoFile = settings.RepositoryConfig
	o.repoCache = settings.RepositoryCache

	//Ensure the file directory exists as it is required for file locking
	err := os.MkdirAll(filepath.Dir(o.repoFile), os.ModePerm)
	if err != nil && !os.IsExist(err) {
		respErr(c, err)
		return
	}

	// Acquire a file lock for process synchronization
	fileLock := flock.New(strings.Replace(o.repoFile, filepath.Ext(o.repoFile), ".lock", 1))
	lockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(lockCtx, time.Second)
	if err == nil && locked {
		defer fileLock.Unlock()
	}
	if err != nil {
		respErr(c, err)
		return
	}

	b, err := ioutil.ReadFile(o.repoFile)
	if err != nil && !os.IsNotExist(err) {
		respErr(c, err)
		return
	}

	var f repo.File
	if err := yaml.Unmarshal(b, &f); err != nil {
		respErr(c, err)
		return
	}

	if o.NoUpdate && f.Has(o.Name) {
		respErr(c, errors.Errorf("repository name (%s) already exists, please specify a different name", o.Name))
		return
	}

	if o.Username != "" && o.Password == "" {
		respErr(c, errors.Errorf("missing password"))
		return
	}

	other := repo.Entry{
		Name:                  o.Name,
		URL:                   o.Url,
		Username:              o.Username,
		Password:              o.Password,
		CertFile:              o.CertFile,
		KeyFile:               o.KeyFile,
		CAFile:                o.CaFile,
		InsecureSkipTLSverify: o.InsecureSkipTLSverify,
	}

	r, err := repo.NewChartRepository(&other, getter.All(settings))
	if err != nil {
		respErr(c, err)
		return
	}

	if o.repoCache != "" {
		r.CachePath = o.repoCache
	}
	if _, err := r.DownloadIndexFile(); err != nil {
		respErr(c, errors.Wrapf(err, "looks like %q is not a valid chart repository or cannot be reached", o.Url))
		return
	}

	f.Update(&other)

	if err := f.WriteFile(o.repoFile, 0644); err != nil {
		respErr(c, err)
		return
	}
	respOK(c, o.Name+" has been added to your repositories\n")
}

type repoAddOptions struct {
	Name     string `json:"name"`
	Url      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	NoUpdate bool   `json:"noUpdate"`

	CertFile              string `json:"certFile"`
	KeyFile               string `json:"keyFile"`
	CaFile                string `json:"caFile"`
	InsecureSkipTLSverify bool   `json:"insecureSkipTLSverify"`

	repoFile  string
	repoCache string
}

// @Summary			删除chart镜像库
// @Description 	通过名称，删除一个镜像库
// @Tags			Repository
// @Param 			reponame path string true "repo1,repo2,repo3..."
// @Success 		200 {object} respBody
// @Router 			/repos/remove/{reponame} [delete]
func removeRepository(c *gin.Context) {
	reponame := c.Param("reponame")
	if reponame == "" {
		respErr(c, fmt.Errorf("chart name can not be empty"))
		return
	}
	names := strings.Split(reponame, ",")

	r, err := repo.LoadFile(settings.RepositoryConfig)
	if os.IsNotExist(err) || len(r.Repositories) == 0 {
		respErr(c, fmt.Errorf("no repositories configured"))
		return
	}

	msg := ""
	for _, name := range names {
		if !r.Remove(name) {
			respErr(c, fmt.Errorf("no repo named %q found", name))
			return
		}
		if err := r.WriteFile(settings.RepositoryConfig, 0644); err != nil {
			respErr(c, err)
			return
		}

		if err := removeRepoCache(settings.RepositoryCache, name); err != nil {
			respErr(c, err)
			return
		}
		msg += name + " has been removed from your repositories\n"
	}

	respOK(c, msg)
}

func removeRepoCache(root, name string) error {
	idx := filepath.Join(root, helmpath.CacheChartsFile(name))
	if _, err := os.Stat(idx); err == nil {
		os.Remove(idx)
	}

	idx = filepath.Join(root, helmpath.CacheIndexFile(name))
	if _, err := os.Stat(idx); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "can't remove index file %s", idx)
	}
	return os.Remove(idx)
}

// @Summary			更新chart镜像库
// @Description 	通过名称，删除一个镜像库
// @Tags			Repository
// @Success 		200 {object} respBody
// @Router 			/repos/update [put]
func updateRepositories(c *gin.Context) {
	type errRepo struct {
		Name string
		Err  string
	}
	errRepoList := []errRepo{}

	var wg sync.WaitGroup
	for _, c := range helmConfig.HelmRepos {
		wg.Add(1)
		go func(c *repo.Entry) {
			defer wg.Done()
			err := updateChart(c)
			if err != nil {
				errRepoList = append(errRepoList, errRepo{
					Name: c.Name,
					Err:  err.Error(),
				})
			}
		}(c)
	}
	wg.Wait()

	if len(errRepoList) > 0 {
		respErr(c, fmt.Errorf("error list: %v", errRepoList))
		return
	}

	respOK(c, nil)
}

func updateChart(c *repo.Entry) error {
	r, err := repo.NewChartRepository(c, getter.All(settings))
	if err != nil {
		return err
	}
	_, err = r.DownloadIndexFile()
	if err != nil {
		return err
	}

	return nil
}
