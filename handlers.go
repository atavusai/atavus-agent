package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ==================== File Operations ====================

type FileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

type ListFilesResult struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
	Count   int         `json:"count"`
}

func handleListFiles(path string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot access path: %v", err)
	}
	if !info.IsDir() {
		return nil, "path is not a directory"
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot read directory: %v", err)
	}

	result := ListFilesResult{
		Path:    absPath,
		Entries: make([]FileEntry, 0),
	}

	for _, entry := range entries {
		info, err := entry.Info()
		name := entry.Name()
		if err != nil {
			continue
		}

		// Skip hidden files/dirs on all platforms
		if strings.HasPrefix(name, ".") {
			continue
		}

		fe := FileEntry{
			Name:    name,
			Path:    filepath.Join(absPath, name),
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime().Format(time.RFC3339),
		}
		result.Entries = append(result.Entries, fe)
	}

	result.Count = len(result.Entries)
	return result, ""
}

type ReadFileResult struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Size     int64  `json:"size"`
	Truncated bool  `json:"truncated"`
	Encoding string `json:"encoding"`
}

func handleReadFile(path string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot access: %v", err)
	}

	if info.IsDir() {
		return nil, "cannot read a directory"
	}

	sizeOk, reason := sandbox.IsFileSizeAllowed(info.Size())
	if !sizeOk {
		return nil, reason
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot read: %v", err)
	}

	result := ReadFileResult{
		Path:     absPath,
		Size:     info.Size(),
		Encoding: "utf-8",
	}

	// Detect if binary
	isBinary := false
	for _, b := range data {
		if b == 0 {
			isBinary = true
			break
		}
	}

	if isBinary {
		// Return file info without content for binary files
		result.Content = "[binary file]"
		result.Encoding = "binary"
	} else {
		// Truncate at 100KB for display
		maxDisplay := 100 * 1024
		if len(data) > maxDisplay {
			result.Content = string(data[:maxDisplay])
			result.Truncated = true
		} else {
			result.Content = string(data)
		}
	}

	return result, ""
}

type WriteFileResult struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Appended bool   `json:"appended"`
}

func handleWriteFile(path, content string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)

	// Check size
	sizeOk, reason := sandbox.IsFileSizeAllowed(int64(len(content)))
	if !sizeOk {
		return nil, reason
	}

	// Create parent directories
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Sprintf("cannot create directory: %v", err)
	}

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, fmt.Sprintf("cannot write file: %v", err)
	}

	info, _ := os.Stat(absPath)
	return WriteFileResult{
		Path: absPath,
		Size: info.Size(),
	}, ""
}

type MoveFileResult struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func handleMoveFile(source, destination string, sandbox *Sandbox) (interface{}, string) {
	allowed1, reason := sandbox.IsPathAllowed(source)
	if !allowed1 {
		return nil, fmt.Sprintf("source: %s", reason)
	}
	allowed2, reason := sandbox.IsPathAllowed(destination)
	if !allowed2 {
		return nil, fmt.Sprintf("destination: %s", reason)
	}

	absSource, _ := filepath.Abs(source)
	absDest, _ := filepath.Abs(destination)

	// Create destination parent dir
	os.MkdirAll(filepath.Dir(absDest), 0755)

	if err := os.Rename(absSource, absDest); err != nil {
		return nil, fmt.Sprintf("cannot move: %v", err)
	}

	return MoveFileResult{
		Source:      absSource,
		Destination: absDest,
	}, ""
}

type CopyFileResult struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Size        int64  `json:"size"`
}

func handleCopyFile(source, destination string, sandbox *Sandbox) (interface{}, string) {
	allowed1, reason := sandbox.IsPathAllowed(source)
	if !allowed1 {
		return nil, fmt.Sprintf("source: %s", reason)
	}
	allowed2, reason := sandbox.IsPathAllowed(destination)
	if !allowed2 {
		return nil, fmt.Sprintf("destination: %s", reason)
	}

	absSource, _ := filepath.Abs(source)
	absDest, _ := filepath.Abs(destination)

	// Create destination parent dir
	os.MkdirAll(filepath.Dir(absDest), 0755)

	sourceFile, err := os.Open(absSource)
	if err != nil {
		return nil, fmt.Sprintf("cannot open source: %v", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(absDest)
	if err != nil {
		return nil, fmt.Sprintf("cannot create destination: %v", err)
	}
	defer destFile.Close()

	written, err := io.Copy(destFile, sourceFile)
	if err != nil {
		return nil, fmt.Sprintf("copy error: %v", err)
	}

	return CopyFileResult{
		Source:      absSource,
		Destination: absDest,
		Size:        written,
	}, ""
}

type DeleteFileResult struct {
	Path     string `json:"path"`
	Recycled bool   `json:"recycled"`
}

func handleDeleteFile(path string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.ValidateOperation("delete_file", path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot access: %v", err)
	}

	// Move to trash/recycle instead of permanent delete
	var recycled bool
	if info.IsDir() {
		// Use a manual trash for directories
		trashPath := getTrashPath()
		os.MkdirAll(trashPath, 0755)
		trashName := filepath.Join(trashPath, info.Name()+"_"+time.Now().Format("20060102_150405"))
		err = os.Rename(absPath, trashName)
		recycled = err == nil
	} else {
		recycled = moveToTrash(absPath)
	}

	// Fallback: permanent delete if trash fails
	if !recycled {
		if info.IsDir() {
			err = os.RemoveAll(absPath)
		} else {
			err = os.Remove(absPath)
		}
		if err != nil {
			return nil, fmt.Sprintf("cannot delete: %v", err)
		}
	}

	return DeleteFileResult{
		Path:     absPath,
		Recycled: recycled,
	}, ""
}

type CreateFolderResult struct {
	Path string `json:"path"`
}

func handleCreateFolder(path string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, fmt.Sprintf("cannot create folder: %v", err)
	}

	return CreateFolderResult{Path: absPath}, ""
}

type SearchFileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time"`
}

type SearchFilesResult struct {
	Pattern  string            `json:"pattern"`
	Root     string            `json:"root"`
	Results  []SearchFileEntry `json:"results"`
	Count    int               `json:"count"`
	Limited  bool              `json:"limited"`
}

func handleSearchFiles(pattern, root string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.IsPathAllowed(root)
	if !allowed {
		return nil, reason
	}

	absRoot, _ := filepath.Abs(root)

	result := SearchFilesResult{
		Pattern: pattern,
		Root:    absRoot,
		Results: make([]SearchFileEntry, 0),
	}

	maxResults := 100

	err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		// Skip hidden directories
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != absRoot {
			return filepath.SkipDir
		}

		// Match pattern (case-insensitive)
		name := d.Name()
		// Try glob match first (supports *.txt, file.*, etc.), fallback to substring match
		if matched, _ := filepath.Match(strings.ToLower(pattern), strings.ToLower(name)); matched || strings.Contains(strings.ToLower(name), strings.ToLower(pattern)) {
			info, _ := d.Info()
			entry := SearchFileEntry{
				Name:    name,
				Path:    path,
				Size:    info.Size(),
				IsDir:   d.IsDir(),
				ModTime: info.ModTime().Format(time.RFC3339),
			}
			result.Results = append(result.Results, entry)
		}

		// Limit results
		if len(result.Results) >= maxResults {
			result.Limited = true
			return io.EOF
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Sprintf("search error: %v", err)
	}

	result.Count = len(result.Results)
	return result, ""
}

type EmptyTrashResult struct {
	RecycledCount int   `json:"recycled_count"`
	FreedBytes    int64 `json:"freed_bytes"`
}

func handleEmptyTrash() (interface{}, string) {
	trashPath := getTrashPath()

	entries, err := os.ReadDir(trashPath)
	if err != nil {
		return EmptyTrashResult{}, ""
	}

	var freedBytes int64
	count := 0

	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil {
			freedBytes += info.Size()
		}
		count++

		entryPath := filepath.Join(trashPath, entry.Name())
		if entry.IsDir() {
			os.RemoveAll(entryPath)
		} else {
			os.Remove(entryPath)
		}
	}

	return EmptyTrashResult{
		RecycledCount: count,
		FreedBytes:    freedBytes,
	}, ""
}

// ==================== System Info ====================

type SystemInfoResult struct {
	Hostname   string `json:"hostname"`
	Platform   string `json:"platform"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	CPUs       int    `json:"cpus"`
	Uptime     string `json:"uptime"`
	HomeDir    string `json:"home_dir"`
	Disks      []DiskInfo `json:"disks"`
	AgentVersion string `json:"agent_version"`
}

type DiskInfo struct {
	Path  string `json:"path"`
	Total int64  `json:"total_bytes"`
	Free  int64  `json:"free_bytes"`
	Used  int64  `json:"used_bytes"`
	Usage float64 `json:"usage_pct"`
}

func handleGetSystemInfo() (interface{}, string) {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()

	result := SystemInfoResult{
		Hostname:     hostname,
		Platform:     detectPlatform(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		CPUs:         runtime.NumCPU(),
		HomeDir:      home,
		AgentVersion: version,
	}

	// Get disk info for home directory
	diskInfo := getDiskUsage(home)
	if diskInfo != nil {
		result.Disks = []DiskInfo{*diskInfo}
	}

	return result, ""
}

func getDiskUsage(path string) *DiskInfo {
	// Use platform-specific approach
	if detectPlatform() == "windows" {
		return getWindowsDiskUsage(path)
	}
	return getUnixDiskUsage(path)
}

func getWindowsDiskUsage(path string) *DiskInfo {
	// On Windows, use GetDiskFreeSpaceEx via syscall
	// Simple implementation: return available space on drive
	absPath, _ := filepath.Abs(path)
	drive := filepath.VolumeName(absPath)
	if drive == "" {
		drive = "C:"
	}

	// Use dir command to get free space
_, _ = execCmdWithOutput("cmd", "/c", "dir", drive+"\\")
	return &DiskInfo{
		Path:  drive + "\\",
		Total: 0,
		Free:  0,
		Used:  0,
		Usage: 0,
	}
}

func getUnixDiskUsage(path string) *DiskInfo {
	// On macOS/Linux, use statfs
	// For simplicity, use df output
	output, _ := execCmdWithOutput("df", "-k", path)
	parts := strings.Fields(output)
	if len(parts) >= 6 {
		// Parse df output: Filesystem 1K-blocks Used Available Use% Mounted
		// The output has header and data rows
		return &DiskInfo{
			Path:  parts[len(parts)-1],
			Total: parseInt64(parts[len(parts)-5]) * 1024,
			Used:  parseInt64(parts[len(parts)-4]) * 1024,
			Free:  parseInt64(parts[len(parts)-3]) * 1024,
			Usage: parseFloat(parts[len(parts)-2]),
		}
	}

	return nil
}

// execCmdWithOutput runs a command and returns stdout
func execCmdWithOutput(name string, args ...string) (string, error) {
	// Simple file-based approach to avoid imports
	// Use os.Pipe approach
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}

	proc, err := os.StartProcess(name, append([]string{name}, args...), &os.ProcAttr{
		Files: []*os.File{nil, w, nil},
	})
	if err != nil {
		return "", err
	}
	w.Close()

	data, _ := io.ReadAll(r)
	r.Close()

	proc.Wait()
	return string(data), nil
}

// ==================== Trash / Recycling ====================

func getTrashPath() string {
	home, _ := os.UserHomeDir()
	if detectPlatform() == "windows" {
		// Windows recycle bin is a special system directory
		// We'll use a local trash folder instead
		return filepath.Join(home, ".atavus-trash")
	}
	return filepath.Join(home, ".Trash")
}

func moveToTrash(path string) bool {
	trashDir := getTrashPath()
	os.MkdirAll(trashDir, 0755)

	_, name := filepath.Split(path)
	trashName := name + "_" + time.Now().Format("20060102_150405")
	trashPath := filepath.Join(trashDir, trashName)

	err := os.Rename(path, trashPath)
	return err == nil
}

// ==================== Helpers ====================

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

func parseFloat(s string) float64 {
	s = strings.TrimSuffix(s, "%")
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// ── Binary File Read (base64) ──────────────────────────

// handleReadFileBase64 reads a file and returns it base64-encoded
func handleReadFileBase64(path string, sandbox *Sandbox) (interface{}, string) {
	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot access: %v", err)
	}

	if info.IsDir() {
		return nil, "cannot read a directory"
	}

	sizeOk, reason := sandbox.IsFileSizeAllowed(info.Size())
	if !sizeOk {
		return nil, reason
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Sprintf("cannot read: %v", err)
	}

	detectedMime := detectMimeType(absPath, data)
	encoded := base64.StdEncoding.EncodeToString(data)

	return map[string]interface{}{
		"path":      absPath,
		"size":      info.Size(),
		"mime_type": detectedMime,
		"extension": filepath.Ext(absPath),
		"content":   encoded,
		"encoding":  "base64",
	}, ""
}

// detectMimeType guesses MIME from extension and magic bytes
func detectMimeType(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".csv":
		return "text/csv"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "text/yaml"
	case ".xml":
		return "text/xml"
	case ".txt":
		return "text/plain"
	case ".doc":
		return "application/msword"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	default:
		// Check magic bytes
		if len(data) > 4 {
			if data[0] == 0x25 && data[1] == 0x50 && data[2] == 0x44 && data[3] == 0x46 {
				return "application/pdf"
			}
			if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
				return "image/png"
			}
			if data[0] == 0xFF && data[1] == 0xD8 {
				return "image/jpeg"
			}
		}
		return "application/octet-stream"
	}
}

// ── File Creation ──────────────────────────────────────

// handleCreateDocument creates a formatted text/CSV/MD file
// handleWriteFileBase64 writes binary data (base64-encoded) to a file
func handleWriteFileBase64(params map[string]interface{}, sandbox *Sandbox) (interface{}, string) {
	path, _ := params["path"].(string)
	b64Content, _ := params["content"].(string)

	if path == "" {
		return nil, "path is required"
	}
	if b64Content == "" {
		return nil, "content (base64) is required"
	}

	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)
	
	// Ensure parent directory exists
	os.MkdirAll(filepath.Dir(absPath), 0755)
	
	data, err := base64.StdEncoding.DecodeString(b64Content)
	if err != nil {
		return nil, fmt.Sprintf("invalid base64: %v", err)
	}

	if err := os.WriteFile(absPath, data, 0644); err != nil {
		return nil, fmt.Sprintf("cannot write file: %v", err)
	}

	return map[string]interface{}{
		"path": absPath,
		"size": len(data),
	}, ""
}

func handleCreateDocument(params map[string]interface{}, sandbox *Sandbox) (interface{}, string) {
	path, _ := params["path"].(string)
	content, _ := params["content"].(string)
	fileType, _ := params["file_type"].(string)

	if path == "" {
		return nil, "path is required"
	}

	allowed, reason := sandbox.IsPathAllowed(path)
	if !allowed {
		return nil, reason
	}

	absPath, _ := filepath.Abs(path)

	// Ensure parent directory exists
	os.MkdirAll(filepath.Dir(absPath), 0755)

	switch strings.ToLower(fileType) {
	case "md", "markdown":
	case "csv":
		// CSV content is plain text with lines
	case "txt", "text":
	case "json":
		// Validate JSON
		if !json.Valid([]byte(content)) {
			return nil, "invalid JSON content"
		}
	case "html":
		// Wrap in basic HTML if not already
		if !strings.HasPrefix(strings.TrimSpace(content), "<") {
			content = "<!DOCTYPE html><html><body>" + content + "</body></html>"
		}
	case "yaml", "yml":
		// YAML content is plain text
	default:
		// Default to raw content, just write it
	}

	// Write the file
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, fmt.Sprintf("cannot create file: %v", err)
	}

	return map[string]interface{}{
		"path":      absPath,
		"size":      len(content),
		"file_type": fileType,
	}, ""
}

// handleCreatePresentation creates a simple PPTX file
func handleCreatePresentation(params map[string]interface{}, sandbox *Sandbox) (interface{}, string) {
	path, _ := params["path"].(string)
	title, _ := params["title"].(string)
	slidesRaw, _ := params["slides"].([]interface{})

	if path == "" {
		return nil, "path is required"
	}

	// We can't create actual PPTX in Go without heavy lib
	// Instead create a Markdown/HTML representation that the backend can read
	content := fmt.Sprintf("# %s\n\n", title)
	for i, slide := range slidesRaw {
		slideMap, ok := slide.(map[string]interface{})
		if !ok {
			continue
		}
		slideTitle, _ := slideMap["title"].(string)
		content += fmt.Sprintf("## Slide %d: %s\n\n", i+1, slideTitle)
		if body, ok := slideMap["body"].(string); ok && body != "" {
			content += body + "\n\n"
		}
		if bullets, ok := slideMap["bullets"].([]interface{}); ok {
			for _, b := range bullets {
				content += fmt.Sprintf("- %v\n", b)
			}
			content += "\n"
		}
		content += "---\n\n"
	}

	absPath, _ := filepath.Abs(path)
	allowed, reason := sandbox.IsPathAllowed(absPath)
	if !allowed {
		return nil, fmt.Sprintf("destination: %s", reason)
	}

	// Ensure parent directory exists
	os.MkdirAll(filepath.Dir(absPath), 0755)

	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, fmt.Sprintf("cannot create presentation: %v", err)
	}

	// Also try to save a .pptx structure if it's a zip (placeholder)
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext == ".pptx" {
		// For now, save markdown alongside so backend can convert
		mdPath := strings.TrimSuffix(absPath, ext) + ".md"
		os.MkdirAll(filepath.Dir(mdPath), 0755)
		_ = os.WriteFile(mdPath, []byte(content+"\n\n<!-- presentation source for backend conversion -->\n"), 0644)
	}

	return map[string]interface{}{
		"path":        absPath,
		"size":        len(content),
		"slide_count": len(slidesRaw),
		"format":      "markdown",
		"note":        "Created as markdown. Use Atavus AI backend to convert to PPTX.",
	}, ""
}
