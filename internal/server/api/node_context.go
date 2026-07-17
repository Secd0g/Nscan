package api

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// serveDockerContext 将源码打包成 tar.gz 供 docker compose build 使用
func (h *Handler) serveDockerContext(c *gin.Context) {
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", "attachment; filename=context.tar.gz")

	gw := gzip.NewWriter(c.Writer)
	tw := tar.NewWriter(gw)

	addFile := func(tarName, srcPath string) error {
		f, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name:    tarName,
			Mode:    int64(info.Mode()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = io.Copy(tw, f)
		return err
	}

	addDir := func(dir, tarPrefix string) error {
		return filepath.Walk(filepath.Join(workDir, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(workDir, path)
			return addFile(filepath.Join(tarPrefix, strings.TrimPrefix(rel, dir+"/")), path)
		})
	}

	// Dockerfile（以 Dockerfile.scanner 为源，打包进 tar 时命名为 Dockerfile）
	_ = addFile("Dockerfile", filepath.Join(workDir, "Dockerfile.scanner"))
	// entrypoint
	_ = addDir("docker", "docker")
	// Go 源码
	_ = addFile("go.mod", filepath.Join(workDir, "go.mod"))
	_ = addFile("go.sum", filepath.Join(workDir, "go.sum"))
	_ = addDir("cmd", "cmd")
	_ = addDir("internal", "internal")
	_ = addDir("pkg", "pkg")

	tw.Close()
	gw.Close()

	c.Status(http.StatusOK)
}
