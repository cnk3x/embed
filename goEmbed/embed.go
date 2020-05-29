package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/tools/imports"
)

type Matcher func(path string, info os.FileInfo) error

type File struct {
	FilePath    string //文件路径
	FileName    string //文件名
	FileSize    int64  //文件大小
	FileModTime int64  //文件修改时间
	Data        string //压缩编码后的字符串
	FileIsDir   bool   //是否文件夹
}

type Option struct {
	Package string //包名
	Name    string //变量名
	Dep     string //是否依赖

	Force    bool    //是否覆盖已存在的目标文件
	Matcher  Matcher //过滤器
	MaxDepth int     //扫描目录最深

	SourcePath string //源路径
	Target     string //目标文件名
	Command    string
}

func (o *Option) Setup() error {
	o.Command = strings.Join(os.Args, " ")
	if o.SourcePath == "" {
		o.SourcePath = "."
	}

	fp, _ := filepath.Abs(o.SourcePath)
	stat, err := os.Stat(fp)
	if err != nil {
		return fmt.Errorf("源路径[ %s ]异常: %w", o.SourcePath, err)
	}

	if stat.IsDir() {
		fileName := filepath.Base(fp)
		folderName := stat.Name()

		if o.Name == "" {
			o.Name = hump(folderName)
		}

		if o.Target == "" {
			o.Target = filepath.Join(o.SourcePath, fileName+".go")
		}
	} else {
		fileName := filepath.Base(fp)
		if o.Name == "" {
			o.Name = hump(fileName)
		}

		if o.Target == "" {
			o.Target = o.SourcePath + ".go"
		}
	}

	target, _ := filepath.Abs(o.Target)
	if o.Package == "" {
		o.Package = underline(filepath.Base(filepath.Dir(target)))
	}

	return nil
}

func (o *Option) ValidateTarget() (bool, error) {
	if o.Target == "-" {
		return true, nil
	}

	stat, err := os.Stat(o.Target)
	if err != nil {
		if os.IsNotExist(err) { //文件不存在，通过
			return true, nil
		}
		return false, fmt.Errorf("检查: 目标路径[ %s ]异常, %w", o.Target, err) //读取文件状态错误，不通过
	}

	//目标是目录，不通过
	if stat.IsDir() {
		return false, fmt.Errorf("检查: 目标[ %s ]是一个目录", o.Target)
	}

	//文件存在，如果不是 force, 不通过
	return o.Force, nil
}

func (o *Option) Process(debug bool) (s string, err error) {
	defer func() {
		if rev := recover(); rev != nil {
			if e, ok := rev.(error); ok {
				err = fmt.Errorf("未处理的错误: %w", e)
			} else {
				err = fmt.Errorf("未处理的错误: %v", rev)
			}
		}
	}()

	var files []*File
	files, err = o.GetFiles()
	if err != nil {
		return "", fmt.Errorf("生成: 编码文件出错, %w", err)
	}

	data := map[string]interface{}{
		"source":   o.SourcePath,
		"target":   o.Target,
		"command":  o.Command,
		"package":  o.Package,
		"dep":      o.Dep,
		"name":     o.Name,
		"fsType":   filepath.Base(o.Dep) + ".Fs",
		"fileType": filepath.Base(o.Dep) + ".File",
		"files":    files,
	}

	w := &bytes.Buffer{}
	if err = template.Must(template.New("go").Parse(goTemplate)).Execute(w, data); err != nil {
		return "", fmt.Errorf("生成: 执行模板出错, %w", err)
	}

	var v []byte
	v, err = imports.Process("", w.Bytes(), nil)
	if err != nil {
		if !debug {
			return "", fmt.Errorf("生成: 格式化出错, %w", err)
		}
		v = w.Bytes()
	}

	if o.Target != "-" {
		if err = ioutil.WriteFile(o.Target, v, 0666); err != nil {
			return "", fmt.Errorf("生成: 写入文件出错, %w", err)
		}
	}

	return string(v), nil
}

func (o *Option) GetFiles() ([]*File, error) {
	matcher := func(path string, info os.FileInfo) error {
		if path == o.Target {
			return filepath.SkipDir
		}
		if info.Name() == ".DS_Store" {
			return filepath.SkipDir
		}
		if err := o.Matcher(path, info); err != nil {
			return err
		}
		return nil
	}

	files := make([]*File, 0, 1)

	err := filepath.Walk(o.SourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}

		relPath, err := filepath.Rel(o.SourcePath, path)
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}

		if relPath == "." {
			relPath = ""
		}

		if err := matcher(filepath.ToSlash(relPath), info); err != nil {
			fmt.Println(err.Error())
			return nil
		}

		if info.IsDir() {
			files = append(files, &File{
				FilePath:    "/" + relPath,
				FileName:    info.Name(),
				FileModTime: info.ModTime().Unix(),
				FileIsDir:   true,
			})
			return nil
		}

		fRes := &File{
			FilePath:    "/" + relPath,
			FileName:    info.Name(),
			FileSize:    info.Size(),
			FileModTime: info.ModTime().Unix(),
		}

		fRes.Data, err = encode(path)
		if err != nil {
			fmt.Println(err.Error())
		} else {
			files = append(files, fRes)
		}
		return nil
	})

	return files, err
}

func encode(fn string) (string, error) {
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return "", err
	}
	if data, err = gzipCompress(data); err != nil {
		return "", err
	}
	return chunkBase64Encode(data, 64), nil
}

func gzipCompress(data []byte) ([]byte, error) {
	w := &bytes.Buffer{}
	gzw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	defer doClose(gzw)
	if _, err = gzw.Write(data); err != nil {
		return nil, err
	}
	if err = gzw.Flush(); err != nil {
		return w.Bytes(), err
	}
	return w.Bytes(), nil
}

func chunkBase64Encode(data []byte, chunkSize int) string {
	v := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
	if base64.StdEncoding.Encode(v, data); len(v) < chunkSize {
		return string(v)
	}

	w := strings.Builder{}
	w.WriteRune('\n')

	chunk := make([]byte, chunkSize)
	r := bytes.NewReader(v)
	for {
		n, _ := r.Read(chunk)
		if n == 0 {
			break
		}
		w.Write(chunk[:n])
		w.WriteRune('\n')
	}

	return w.String()
}

func doClose(closer io.Closer) {
	_ = closer.Close()
}

//驼峰命令
func hump(src string) string {
	var (
		out    = make([]rune, 0, len(src))
		needUp = true
	)
	for _, n := range src {
		if ('A' <= n && n <= 'Z') || ('a' <= n && n <= 'z') {
			if needUp && 'a' <= n && n <= 'z' {
				needUp = false
				n -= 'a' - 'A'
			}
			out = append(out, n)
		} else {
			needUp = true
		}
	}
	return string(out)
}

//下划线命令
func underline(src string) string {
	var (
		out    = make([]rune, 0, len(src))
		needUp = false
		lastUp = false
	)
	for _, n := range src {
		if ('A' <= n && n <= 'Z') || ('a' <= n && n <= 'z') {
			if 'A' <= n && n <= 'Z' {
				n += 'a' - 'A'
				if !lastUp {
					needUp = true
				}
			}
			if needUp {
				lastUp = true
				needUp = false
				out = append(out, '_')
			}
			out = append(out, n)
		} else {
			if !lastUp {
				needUp = true
			}
		}
	}
	return string(out)
}

const goTemplate = `// Code generated by goEmbed (github.com/shuxs/embed/goEmbed); DO NOT EDIT.
// Source: {{ .source }}
// Target: {{ .target }}
// Command: {{ .command }}
package {{ .package }}

import (
    "{{ .dep }}"
)

{{ $model := . }}

var {{ .name }} = {{ .fsType }} {
   	{{ range $file := .files -}}
   	"{{ $file.FilePath }}": &{{ $model.fileType }} {
   	    FileName:        "{{ $file.FileName }}",
   	    FileModTime:      {{ $file.FileModTime }},
   	    {{ if $file.FileIsDir -}}
   	    FileIsDir:        true,
   	    {{- else -}}
   	    FileSize:         {{ $file.FileSize }},
   	    Data: ` + "`{{ $file.Data }}`," + `
   	    {{- end }}
   	},
   	{{ end }}
}
`
