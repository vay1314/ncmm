// Copyright (c) 2026 @3899. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package ncmm

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/3899/ncmm/config"
	"github.com/3899/ncmm/pkg/log"
	"github.com/3899/ncmm/pkg/utils"
)

var (
	updaterMu          sync.Mutex
	hasUpdate          bool
	latestVersionStr   string
	autoUpdateStatus   string // "idle", "success", "failed"
	autoUpdateError    string
)

type UpdateCache struct {
	LastCheckTime time.Time `json:"last_check_time"`
	LatestVersion string    `json:"latest_version"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// getHomePath 获取工作目录家目录
func (c *Root) getHomePath() string {
	if c.Opts.Home != "" {
		return filepath.Clean(c.Opts.Home)
	}
	return filepath.Clean(config.HomeDir)
}

// CleanOldExecutable 清理上一次更新留下的 .old 临时备份文件
func (c *Root) CleanOldExecutable() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	oldPath := execPath + ".old"
	if utils.FileExists(oldPath) {
		_ = os.Remove(oldPath)
	}
}

// CheckForUpdatesPreRun 执行 PreRun 阶段的缓存版本校验与低频异步检测
func (c *Root) CheckForUpdatesPreRun() {
	c.CleanOldExecutable()

	// 获取配置中的 updater 控制参数，若未配置则使用默认值 (check=true, auto_update=false)
	checkEnabled := true
	autoUpdateEnabled := false
	if c.Cfg != nil && c.Cfg.Updater != nil {
		if c.Cfg.Updater.Check != nil {
			checkEnabled = *c.Cfg.Updater.Check
		}
		if c.Cfg.Updater.AutoUpdate != nil {
			autoUpdateEnabled = *c.Cfg.Updater.AutoUpdate
		}
	}

	if !checkEnabled {
		return
	}

	// 支持环境变量临时关闭
	if os.Getenv("NCMM_NO_UPDATE_CHECK") == "1" || os.Getenv("NO_UPDATE_CHECK") == "1" {
		return
	}

	home := c.getHomePath()
	cachePath := filepath.Join(home, "update_cache.json")
	var cache UpdateCache

	// 读取缓存
	if utils.FileExists(cachePath) {
		if data, err := os.ReadFile(cachePath); err == nil {
			_ = json.Unmarshal(data, &cache)
		}
	}

	currentVer := c.AppVersion
	if currentVer == "" {
		currentVer = "0.0.0"
	}

	// 缓存命中提醒：如果缓存的最新版本大于当前版本，立刻标记为需要提醒
	if cache.LatestVersion != "" && CompareVersions(currentVer, cache.LatestVersion) < 0 {
		updaterMu.Lock()
		hasUpdate = true
		latestVersionStr = cache.LatestVersion
		updaterMu.Unlock()
	}

	// 检测频率控制：如果距离上一次检测不足 24 小时，不再请求网络
	if time.Since(cache.LastCheckTime) < 24*time.Hour {
		return
	}

	// 异步发起 GitHub API 检测并处理自动更新
	go c.checkNewVersionAsync(cachePath, currentVer, autoUpdateEnabled)
}

func (c *Root) checkNewVersionAsync(cachePath string, currentVer string, autoUpdateEnabled bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/3899/ncmm/releases/latest", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "ncmm-updater/"+currentVer)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return
	}

	tag := strings.TrimSpace(rel.TagName)
	if tag == "" {
		return
	}

	// 更新缓存时间与版本号
	cache := UpdateCache{
		LastCheckTime: time.Now(),
		LatestVersion: tag,
	}
	if data, err := json.MarshalIndent(cache, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
		_ = os.WriteFile(cachePath, data, 0644)
	}

	// 比对新版本
	if CompareVersions(currentVer, tag) >= 0 {
		return // 已经是最新版，无需更新
	}

	updaterMu.Lock()
	hasUpdate = true
	latestVersionStr = tag
	updaterMu.Unlock()

	// 自动更新逻辑（触发条件：开启 auto_update & 非官方容器环境）
	isDockerOfficial := os.Getenv("NCMM_DOCKER_OFFICIAL") == "1"
	if autoUpdateEnabled && !isDockerOfficial {
		autoUpdateStatus = "updating"
		if err := c.performSelfUpdate(tag, rel.Assets); err != nil {
			autoUpdateStatus = "failed"
			autoUpdateError = err.Error()
			log.Error("[updater] 自动更新失败: %s", err)
		} else {
			autoUpdateStatus = "success"
			log.Info("[updater] 自动热更新成功完成，新版本将自下次运行生效。")
		}
	}
}

// performSelfUpdate 执行自动热更新流程：下载、提取、重命名及物理替换
func (c *Root) performSelfUpdate(tag string, assets []githubAsset) error {
	osPart, archPart, ext := getPlatformInfo()
	targetName := fmt.Sprintf("ncmm_%s_%s%s", osPart, archPart, ext)

	var downloadURL string
	for _, asset := range assets {
		if strings.EqualFold(asset.Name, targetName) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		// 手动拼接备用 URL
		downloadURL = fmt.Sprintf("https://github.com/3899/ncmm/releases/download/%s/%s", tag, targetName)
	}

	// 代理处理
	downloadURL = proxyURL(downloadURL)

	// 下载升级包
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ncmm-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status error: %d", resp.StatusCode)
	}

	archiveBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// 提取包中的二进制数据
	var binaryBytes []byte
	binaryName := "ncmm"
	if runtime.GOOS == "windows" {
		binaryName = "ncmm.exe"
	}

	if ext == ".zip" {
		zipReader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
		if err != nil {
			return fmt.Errorf("zip.NewReader: %w", err)
		}
		for _, file := range zipReader.File {
			if strings.EqualFold(filepath.Base(file.Name), binaryName) {
				rc, err := file.Open()
				if err != nil {
					return err
				}
				binaryBytes, err = io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return err
				}
				break
			}
		}
	} else {
		// tar.gz
		gzipReader, err := gzip.NewReader(bytes.NewReader(archiveBytes))
		if err != nil {
			return fmt.Errorf("gzip.NewReader: %w", err)
		}
		defer gzipReader.Close()

		tarReader := tar.NewReader(gzipReader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if strings.EqualFold(filepath.Base(header.Name), binaryName) {
				binaryBytes, err = io.ReadAll(tarReader)
				if err != nil {
					return err
				}
				break
			}
		}
	}

	if len(binaryBytes) == 0 {
		return fmt.Errorf("在升级包中找不到可执行文件: %s", binaryName)
	}

	// 获取当前运行的可执行程序路径
	execPath, err := os.Executable()
	if err != nil {
		return err
	}

	// 重命名活动文件释放锁定路径
	oldPath := execPath + ".old"
	if utils.FileExists(oldPath) {
		_ = os.Remove(oldPath)
	}
	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("重命名原程序失败: %w", err)
	}

	// 将解压出的新二进制文件写入原路径
	if err := os.WriteFile(execPath, binaryBytes, 0755); err != nil {
		// 写入失败时尝试将备份移回
		_ = os.Rename(oldPath, execPath)
		return fmt.Errorf("写入新二进制文件失败: %w", err)
	}

	// 执行配置文件升级
	if c.CfgPath != "" && c.CfgPath != "default" {
		if err := config.AutoUpgradeConfig(c.CfgPath); err != nil {
			log.Warn("[updater] 配置文件自动升级合并失败: %s", err)
		}
	}

	return nil
}

// ShowUpdateNotificationPostRun 在 PersistentPostRun 阶段渲染终端升级提示
func (c *Root) ShowUpdateNotificationPostRun() {
	updaterMu.Lock()
	show := hasUpdate
	ver := latestVersionStr
	status := autoUpdateStatus
	errStr := autoUpdateError
	updaterMu.Unlock()

	if !show {
		return
	}

	isDockerOfficial := os.Getenv("NCMM_DOCKER_OFFICIAL") == "1"

	fmt.Println()
	fmt.Println("==============================================================")
	fmt.Printf("📢  检测到 ncmm 有新版本发布: %s (当前版本: %s)\n", ver, c.AppVersion)
	fmt.Println("--------------------------------------------------------------")

	if isDockerOfficial {
		fmt.Println("🐳  由于检测到在官方 Docker 容器中运行，已跳过二进制自动替换。")
		fmt.Println("👉  请在宿主机执行以下命令升级容器镜像：")
		fmt.Println("    docker pull ghcr.io/3899/ncmm:latest")
	} else {
		switch status {
		case "success":
			fmt.Println("✨  新版本二进制和配置文件已在后台自动下载并热替换完成！")
			fmt.Println("👉  当前进程运行结束后，下一次启动即刻生效新版本。")
		case "failed":
			fmt.Printf("⚠️  自动下载更新失败: %s\n", errStr)
			fmt.Println("👉  您可以直接访问 GitHub 下载最新发布版本进行手动升级：")
			fmt.Println("    https://github.com/3899/ncmm/releases")
		default:
			fmt.Println("👉  您可以直接访问 GitHub 下载最新发布版本进行手动升级：")
			fmt.Println("    https://github.com/3899/ncmm/releases")
		}
	}
	fmt.Println("==============================================================")
}

// CompareVersions 比对版本号。v1 > v2 返回 1，v1 < v2 返回 -1，相等返回 0
func CompareVersions(v1, v2 string) int {
	p1 := parseVersion(v1)
	p2 := parseVersion(v2)

	for i := 0; i < len(p1) || i < len(p2); i++ {
		var val1, val2 int
		if i < len(p1) {
			val1 = p1[i]
		}
		if i < len(p2) {
			val2 = p2[i]
		}
		if val1 < val2 {
			return -1
		} else if val1 > val2 {
			return 1
		}
	}
	return 0
}

func parseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	parts := strings.Split(v, ".")
	res := make([]int, 0, len(parts))
	for _, p := range parts {
		var digits string
		for _, r := range p {
			if r >= '0' && r <= '9' {
				digits += string(r)
			} else {
				break
			}
		}
		if digits != "" {
			val, _ := strconv.Atoi(digits)
			res = append(res, val)
		} else {
			res = append(res, 0)
		}
	}
	return res
}

func getPlatformInfo() (string, string, string) {
	osPart := runtime.GOOS
	switch runtime.GOOS {
	case "windows":
		osPart = "Windows"
	case "linux":
		osPart = "Linux"
	case "darwin":
		osPart = "Darwin"
	default:
		if len(osPart) > 0 {
			osPart = strings.ToUpper(osPart[:1]) + osPart[1:]
		}
	}

	archPart := runtime.GOARCH
	switch runtime.GOARCH {
	case "amd64":
		archPart = "x86_64"
	case "arm64":
		archPart = "arm64"
	case "arm":
		archPart = "armv6"
	}

	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}

	return osPart, archPart, ext
}

func proxyURL(url string) string {
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "https://github.com/") || strings.HasPrefix(url, "https://raw.githubusercontent.com/") {
		return fmt.Sprintf("https://gh-proxy.com/%s", url)
	}
	return url
}
