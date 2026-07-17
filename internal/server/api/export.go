package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"github.com/yourname/nscan/internal/server/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ExportAssets 导出资产到 Excel
// GET /api/export/assets?type=http|port|subdomain|vuln|dir|sensitive&project_id=&task_id=
func (h *Handler) ExportAssets(c *gin.Context) {
	assetType := c.Query("type")
	if assetType == "" {
		errResp(c, http.StatusBadRequest, "type required")
		return
	}

	f := repositories.AssetFilter{
		TaskID:    c.Query("task_id"),
		ProjectID: c.Query("project_id"),
		Q:         c.Query("q"),
		Limit:     10000,
	}

	xlsx := excelize.NewFile()
	defer xlsx.Close()

	var err error
	switch assetType {
	case "http":
		err = exportHTTP(c, xlsx, h, f)
	case "port":
		err = exportPort(c, xlsx, h, f)
	case "subdomain":
		err = exportSubdomain(c, xlsx, h, f)
	case "vuln":
		err = exportVuln(c, xlsx, h, f)
	case "dir":
		err = exportDir(c, xlsx, h, f)
	case "sensitive":
		err = exportSensitive(c, xlsx, h, f)
	default:
		errResp(c, http.StatusBadRequest, "unknown type: "+assetType)
		return
	}

	if err != nil {
		errResp(c, http.StatusInternalServerError, err.Error())
		return
	}

	filename := fmt.Sprintf("nscan_%s_%s.xlsx", assetType, time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := xlsx.Write(c.Writer); err != nil {
		h.log.Sugar().Errorf("export write: %v", err)
	}
}

func setHeader(f *excelize.File, sheet string, headers []string) {
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#E8F4FF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
		_ = f.SetCellStyle(sheet, cell, cell, style)
	}
}

func writeRow(f *excelize.File, sheet string, row int, vals []interface{}) {
	for i, v := range vals {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		_ = f.SetCellValue(sheet, cell, v)
	}
}

func exportHTTP(c *gin.Context, f *excelize.File, h *Handler, filter repositories.AssetFilter) error {
	sheet := "HTTP资产"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"URL", "域名", "IP", "端口", "状态码", "标题", "技术栈", "内容长度", "来源", "发现时间"}
	setHeader(f, sheet, headers)

	rows, _, err := h.assets.ListHTTP(c.Request.Context(), filter)
	if err != nil {
		return err
	}
	for i, a := range rows {
		writeRow(f, sheet, i+2, []interface{}{
			a.URL, a.Domain, a.IP, a.Port, a.StatusCode,
			a.Title, strings.Join(a.Tech, ","), a.ContentLen,
			a.Source, a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	autoWidth(f, sheet, len(headers))
	return nil
}

func exportPort(c *gin.Context, f *excelize.File, h *Handler, filter repositories.AssetFilter) error {
	sheet := "端口资产"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"IP", "端口", "协议", "状态", "服务", "Banner", "来源", "发现时间"}
	setHeader(f, sheet, headers)

	rows, _, err := h.assets.ListPorts(c.Request.Context(), filter)
	if err != nil {
		return err
	}
	for i, a := range rows {
		writeRow(f, sheet, i+2, []interface{}{
			a.IP, a.Port, a.Protocol, a.State, a.Service,
			a.Banner, strings.Join(a.Sources, ","), a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	autoWidth(f, sheet, len(headers))
	return nil
}

func exportSubdomain(c *gin.Context, f *excelize.File, h *Handler, filter repositories.AssetFilter) error {
	sheet := "子域名"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"域名", "IP列表", "来源", "发现时间"}
	setHeader(f, sheet, headers)

	rows, _, err := h.assets.ListSubdomains(c.Request.Context(), filter)
	if err != nil {
		return err
	}
	for i, a := range rows {
		writeRow(f, sheet, i+2, []interface{}{
			a.Domain, strings.Join(a.IPs, ","),
			strings.Join(a.Sources, ","), a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	autoWidth(f, sheet, len(headers))
	return nil
}

func exportVuln(c *gin.Context, f *excelize.File, h *Handler, filter repositories.AssetFilter) error {
	sheet := "漏洞"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"目标", "漏洞名称", "模板ID", "危险等级", "匹配位置", "标签", "发现时间"}
	setHeader(f, sheet, headers)

	// severity color styles
	severityStyle := map[string]string{
		"critical": "FF0000",
		"high":     "FF6600",
		"medium":   "FFAA00",
		"low":      "3399FF",
		"info":     "888888",
	}
	styleCache := map[string]int{}
	for sev, color := range severityStyle {
		s, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Color: color, Bold: sev == "critical" || sev == "high"},
		})
		styleCache[sev] = s
	}

	rows, _, err := h.assets.ListVulns(c.Request.Context(), filter)
	if err != nil {
		return err
	}
	for i, a := range rows {
		row := i + 2
		writeRow(f, sheet, row, []interface{}{
			a.Target, a.Name, a.TemplateID, a.Severity,
			a.MatchedAt, "",
			a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
		// color severity cell (column 4)
		if sid, ok := styleCache[strings.ToLower(a.Severity)]; ok {
			cell, _ := excelize.CoordinatesToCellName(4, row)
			_ = f.SetCellStyle(sheet, cell, cell, sid)
		}
	}
	autoWidth(f, sheet, len(headers))
	return nil
}

func exportDir(c *gin.Context, f *excelize.File, h *Handler, filter repositories.AssetFilter) error {
	sheet := "目录扫描"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"URL", "路径", "状态码", "内容长度", "内容类型", "跳转地址", "发现时间"}
	setHeader(f, sheet, headers)

	rows, _, err := h.assets.ListDirs(c.Request.Context(), filter)
	if err != nil {
		return err
	}
	for i, a := range rows {
		writeRow(f, sheet, i+2, []interface{}{
			a.URL, a.Path, a.StatusCode, a.ContentLen,
			a.ContentType, a.RedirectURL,
			a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	autoWidth(f, sheet, len(headers))
	return nil
}

func exportSensitive(c *gin.Context, f *excelize.File, h *Handler, filter repositories.AssetFilter) error {
	sheet := "敏感信息"
	f.SetSheetName("Sheet1", sheet)
	headers := []string{"URL", "规则名", "匹配内容", "发现时间"}
	setHeader(f, sheet, headers)

	rows, _, err := h.assets.ListSensitive(c.Request.Context(), filter)
	if err != nil {
		return err
	}
	for i, a := range rows {
		writeRow(f, sheet, i+2, []interface{}{
			a.URL, a.RuleName, a.Matched,
			a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	autoWidth(f, sheet, len(headers))
	return nil
}

// autoWidth 为前 n 列设置合理的列宽
func autoWidth(f *excelize.File, sheet string, n int) {
	widths := []float64{40, 25, 15, 10, 10, 30, 20, 12, 12, 18, 18, 18, 18, 18}
	for i := 0; i < n && i < len(widths); i++ {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, widths[i])
	}
}

// ExportAllAssets 统一导出所有类型资产到多 Sheet Excel
// GET /api/export/assets/all?project_id=&task_id=
func (h *Handler) ExportAllAssets(c *gin.Context) {
	filter := repositories.AssetFilter{
		TaskID:    c.Query("task_id"),
		ProjectID: c.Query("project_id"),
		Limit:     10000,
	}

	f := excelize.NewFile()
	defer f.Close()
	ctx := c.Request.Context()

	type sheetDef struct {
		name   string
		export func() error
	}

	sheets := []sheetDef{
		{"资产", func() error {
			rows, _, err := h.assets.ListHTTP(ctx, filter)
			if err != nil {
				return err
			}
			f.SetSheetName("Sheet1", "资产")
			setHeader(f, "资产", []string{"URL", "域名", "IP", "端口", "状态码", "标题", "技术栈", "内容长度", "来源", "发现时间"})
			for i, a := range rows {
				writeRow(f, "资产", i+2, []interface{}{
					a.URL, a.Domain, a.IP, a.Port, a.StatusCode, a.Title,
					strings.Join(a.Tech, ","), a.ContentLen, a.Source, a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "资产", 10)
			return nil
		}},
		{"IP/端口", func() error {
			rows, _, err := h.assets.ListPorts(ctx, filter)
			if err != nil {
				return err
			}
			f.NewSheet("IP/端口")
			setHeader(f, "IP/端口", []string{"IP", "端口", "协议", "状态", "服务", "Banner", "来源", "发现时间"})
			for i, a := range rows {
				writeRow(f, "IP/端口", i+2, []interface{}{
					a.IP, a.Port, a.Protocol, a.State, a.Service, a.Banner, strings.Join(a.Sources, ","), a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "IP/端口", 8)
			return nil
		}},
		{"子域名", func() error {
			rows, _, err := h.assets.ListSubdomains(ctx, filter)
			if err != nil {
				return err
			}
			f.NewSheet("子域名")
			setHeader(f, "子域名", []string{"域名", "IP列表", "来源", "发现时间"})
			for i, a := range rows {
				writeRow(f, "子域名", i+2, []interface{}{
					a.Domain, strings.Join(a.IPs, ","), strings.Join(a.Sources, ","), a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "子域名", 4)
			return nil
		}},
		{"漏洞", func() error {
			rows, _, err := h.assets.ListVulns(ctx, filter)
			if err != nil {
				return err
			}
			f.NewSheet("漏洞")
			setHeader(f, "漏洞", []string{"目标", "漏洞名称", "模板ID", "危险等级", "匹配位置", "标签", "发现时间"})
			for i, a := range rows {
				writeRow(f, "漏洞", i+2, []interface{}{
					a.Target, a.Name, a.TemplateID, a.Severity,
					a.MatchedAt, "", a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "漏洞", 7)
			return nil
		}},
		{"目录", func() error {
			rows, _, err := h.assets.ListDirs(ctx, filter)
			if err != nil {
				return err
			}
			f.NewSheet("目录")
			setHeader(f, "目录", []string{"URL", "路径", "状态码", "内容长度", "内容类型", "跳转地址", "发现时间"})
			for i, a := range rows {
				writeRow(f, "目录", i+2, []interface{}{
					a.URL, a.Path, a.StatusCode, a.ContentLen, a.ContentType, a.RedirectURL, a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "目录", 7)
			return nil
		}},
		{"敏感信息", func() error {
			rows, _, err := h.assets.ListSensitive(ctx, filter)
			if err != nil {
				return err
			}
			f.NewSheet("敏感信息")
			setHeader(f, "敏感信息", []string{"URL", "规则名", "匹配内容", "发现时间"})
			for i, a := range rows {
				writeRow(f, "敏感信息", i+2, []interface{}{
					a.URL, a.RuleName, a.Matched, a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "敏感信息", 4)
			return nil
		}},
	}

	for _, s := range sheets {
		if err := s.export(); err != nil {
			errResp(c, http.StatusInternalServerError, s.name+": "+err.Error())
			return
		}
	}

	filename := fmt.Sprintf("nscan_assets_%s.xlsx", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		h.log.Sugar().Errorf("export all assets write: %v", err)
	}
}

// ExportTaskReport 导出单个任务的综合报告（多 Sheet）
// GET /api/export/task/:id
func (h *Handler) ExportTaskReport(c *gin.Context) {
	taskID := c.Param("id")
	if _, err := primitive.ObjectIDFromHex(taskID); err != nil {
		errResp(c, http.StatusBadRequest, "invalid task id")
		return
	}

	f := excelize.NewFile()
	defer f.Close()

	ctx := c.Request.Context()

	baseFilter := repositories.AssetFilter{TaskID: taskID, Limit: 10000}

	type sheetDef struct {
		name   string
		export func() error
	}

	sheets := []sheetDef{
		{"子域名", func() error {
			rows, _, err := h.assets.ListSubdomains(ctx, baseFilter)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}
			f.NewSheet("子域名")
			setHeader(f, "子域名", []string{"域名", "IP列表", "来源", "发现时间"})
			for i, a := range rows {
				writeRow(f, "子域名", i+2, []interface{}{
					a.Domain, strings.Join(a.IPs, ","), strings.Join(a.Sources, ","), a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "子域名", 4)
			return nil
		}},
		{"端口", func() error {
			rows, _, err := h.assets.ListPorts(ctx, baseFilter)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}
			f.NewSheet("端口")
			setHeader(f, "端口", []string{"IP", "端口", "协议", "状态", "服务", "Banner", "来源", "发现时间"})
			for i, a := range rows {
				writeRow(f, "端口", i+2, []interface{}{
					a.IP, a.Port, a.Protocol, a.State, a.Service, a.Banner, strings.Join(a.Sources, ","), a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "端口", 8)
			return nil
		}},
		{"HTTP资产", func() error {
			rows, _, err := h.assets.ListHTTP(ctx, baseFilter)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}
			f.NewSheet("HTTP资产")
			setHeader(f, "HTTP资产", []string{"URL", "域名", "IP", "端口", "状态码", "标题", "技术栈", "内容长度", "来源", "发现时间"})
			for i, a := range rows {
				writeRow(f, "HTTP资产", i+2, []interface{}{
					a.URL, a.Domain, a.IP, a.Port, a.StatusCode, a.Title,
					strings.Join(a.Tech, ","), a.ContentLen, a.Source, a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "HTTP资产", 10)
			return nil
		}},
		{"漏洞", func() error {
			rows, _, err := h.assets.ListVulns(ctx, baseFilter)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}
			f.NewSheet("漏洞")
			setHeader(f, "漏洞", []string{"目标", "漏洞名称", "模板ID", "危险等级", "匹配位置", "标签", "发现时间"})
			for i, a := range rows {
				writeRow(f, "漏洞", i+2, []interface{}{
					a.Target, a.Name, a.TemplateID, a.Severity,
					a.MatchedAt, "", a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "漏洞", 7)
			return nil
		}},
		{"敏感信息", func() error {
			rows, _, err := h.assets.ListSensitive(ctx, baseFilter)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}
			f.NewSheet("敏感信息")
			setHeader(f, "敏感信息", []string{"URL", "规则名", "匹配内容", "发现时间"})
			for i, a := range rows {
				writeRow(f, "敏感信息", i+2, []interface{}{
					a.URL, a.RuleName, a.Matched, a.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}
			autoWidth(f, "敏感信息", 4)
			return nil
		}},
	}

	// remove default Sheet1
	f.DeleteSheet("Sheet1")

	for _, s := range sheets {
		if err := s.export(); err != nil {
			errResp(c, http.StatusInternalServerError, s.name+": "+err.Error())
			return
		}
	}

	filename := fmt.Sprintf("nscan_task_%s_%s.xlsx", taskID[:8], time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err := f.Write(c.Writer); err != nil {
		h.log.Sugar().Errorf("export task report write: %v", err)
	}
}
