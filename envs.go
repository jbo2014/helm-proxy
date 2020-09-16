package main

import "github.com/gin-gonic/gin"

// @Summary 	获取helm环境信息
// @Description 获取helm环境信息
// @Tags		Env
// @Success 	200 {object} respBody
// @Router 		/envs [get]
func getHelmEnvs(c *gin.Context) {
	respOK(c, settings.EnvVars())
}
