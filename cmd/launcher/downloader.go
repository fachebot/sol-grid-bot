package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubAPIBaseURL = "https://api.github.com"
	repoOwner        = "fachebot"
	repoName         = "sol-grid-bot"
)

// ReleaseInfo GitHub Release信息
type ReleaseInfo struct {
	TagName     string  `json:"tag_name"`
	Assets      []Asset `json:"assets"`
	Draft       bool    `json:"draft"`        // 是否为草稿
	Prerelease  bool    `json:"prerelease"`   // 是否为预发布版本
	PublishedAt string  `json:"published_at"` // 发布时间
	CreatedAt   string  `json:"created_at"`   // 创建时间
}

// Asset Release资源文件
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Downloader GitHub Release下载器
type Downloader struct {
	client *http.Client
}

// NewDownloader 创建下载器
func NewDownloader() *Downloader {
	return &Downloader{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TagInfo GitHub Tag信息
type TagInfo struct {
	Name       string `json:"name"`
	ZipballURL string `json:"zipball_url"`
	TarballURL string `json:"tarball_url"`
	Commit     struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
}

// GetLatestRelease 获取最新release版本
func (d *Downloader) GetLatestRelease(ctx context.Context) (*ReleaseInfo, error) {
	// 使用 GitHub API: GET /repos/{owner}/{repo}/releases/latest
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPIBaseURL, repoOwner, repoName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sol-grid-bot-deploy/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(bodyBytes)
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, bodyStr)
	}

	var release ReleaseInfo
	if err := json.Unmarshal(bodyBytes, &release); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return &release, nil
}

// GetReleaseByTag 通过tag名称获取特定release
func (d *Downloader) GetReleaseByTag(ctx context.Context, tagName string) (*ReleaseInfo, error) {
	// GitHub API: GET /repos/{owner}/{repo}/releases/tags/{tag}
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", githubAPIBaseURL, repoOwner, repoName, tagName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sol-grid-bot-deploy/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("未找到tag为 %s 的release: %s", tagName, string(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}

	var release ReleaseInfo
	if err := json.Unmarshal(bodyBytes, &release); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	return &release, nil
}

// GetReleaseByTagFromList 从release列表中通过tag名称查找release
func (d *Downloader) GetReleaseByTagFromList(ctx context.Context, tagName string) (*ReleaseInfo, error) {
	// 获取所有releases列表
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", githubAPIBaseURL, repoOwner, repoName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sol-grid-bot-deploy/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}

	var releases []ReleaseInfo
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	if err := json.Unmarshal(bodyBytes, &releases); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 遍历列表查找匹配的tag（不区分大小写）
	for _, release := range releases {
		if strings.EqualFold(release.TagName, tagName) {
			return &release, nil
		}
	}

	return nil, fmt.Errorf("未找到tag为 %s 的release", tagName)
}

// GetAssetForCurrentPlatform 获取当前平台对应的资源文件
func (d *Downloader) GetAssetForCurrentPlatform(release *ReleaseInfo) (*Asset, error) {
	var assetName string
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// 构建期望的文件名模式
	if osName == "windows" {
		assetName = fmt.Sprintf("sol-grid-bot-%s-windows-%s.zip", release.TagName, arch)
	} else {
		assetName = fmt.Sprintf("sol-grid-bot-%s-linux-%s.tar.gz", release.TagName, arch)
	}

	// 查找匹配的资源
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			return &asset, nil
		}
	}

	// 如果精确匹配失败，尝试模糊匹配
	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, osName) && strings.Contains(asset.Name, arch) {
			return &asset, nil
		}
	}

	return nil, fmt.Errorf("未找到适合当前平台(%s/%s)的资源文件", osName, arch)
}

// DownloadAsset 下载资源文件
func (d *Downloader) DownloadAsset(ctx context.Context, asset *Asset, destDir string, progressCallback func(current, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	// 创建临时文件
	tmpFile := filepath.Join(destDir, asset.Name)
	out, err := os.Create(tmpFile)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %w", err)
	}
	defer out.Close()

	// 下载并显示进度
	total := asset.Size
	var current int64
	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			written, writeErr := out.Write(buf[:n])
			if writeErr != nil {
				return "", fmt.Errorf("写入文件失败: %w", writeErr)
			}
			current += int64(written)
			if progressCallback != nil {
				progressCallback(current, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("读取数据失败: %w", err)
		}
	}

	return tmpFile, nil
}

// ExtractFile 解压文件
func (d *Downloader) ExtractFile(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return d.extractZip(archivePath, destDir)
	} else if strings.HasSuffix(archivePath, ".tar.gz") {
		return d.extractTarGz(archivePath, destDir)
	}
	return "", fmt.Errorf("不支持的文件格式")
}

// extractZip 解压ZIP文件
func (d *Downloader) extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("打开ZIP文件失败: %w", err)
	}
	defer r.Close()

	var exePath string
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}

		// 查找可执行文件
		if strings.HasSuffix(f.Name, ".exe") || (!strings.Contains(f.Name, ".") && runtime.GOOS != "windows") {
			rc, err := f.Open()
			if err != nil {
				continue
			}

			exePath = filepath.Join(destDir, filepath.Base(f.Name))
			out, err := os.Create(exePath)
			if err != nil {
				rc.Close()
				continue
			}

			_, err = io.Copy(out, rc)
			rc.Close()
			out.Close()

			if err != nil {
				os.Remove(exePath)
				continue
			}

			// 设置执行权限（Linux/Mac）
			if runtime.GOOS != "windows" {
				os.Chmod(exePath, 0755)
			}
		}
	}

	if exePath == "" {
		return "", fmt.Errorf("未找到可执行文件")
	}

	return exePath, nil
}

// extractTarGz 解压tar.gz文件
func (d *Downloader) extractTarGz(archivePath, destDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("创建gzip读取器失败: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	var exePath string

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("读取tar文件失败: %w", err)
		}

		if header.Typeflag == tar.TypeReg {
			// 查找可执行文件
			name := filepath.Base(header.Name)
			if !strings.Contains(name, ".") || strings.HasSuffix(name, ".exe") {
				exePath = filepath.Join(destDir, name)
				out, err := os.Create(exePath)
				if err != nil {
					continue
				}

				_, err = io.Copy(out, tarReader)
				out.Close()

				if err != nil {
					os.Remove(exePath)
					continue
				}

				os.Chmod(exePath, 0755)
				break
			}
		}
	}

	if exePath == "" {
		return "", fmt.Errorf("未找到可执行文件")
	}

	return exePath, nil
}
