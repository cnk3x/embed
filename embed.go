package embed

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

/* http.FileSystem */
var _ http.FileSystem = Fs(nil)

type Fs map[string]*File

func (fs Fs) Open(name string) (http.File, error) {
	stat, err := fs.ReadBytes(name)
	if err != nil {
		return nil, err
	}
	return &httpFile{reader: bytes.NewReader(stat.data), stat: stat}, nil
}

func (fs Fs) ReadBytes(name string) (*File, error) {
	stat := fs[name]
	if stat == nil {
		return nil, os.ErrNotExist
	}

	if stat.FileSize > 0 && len(stat.data) == 0 {
		gzr, err := gzip.NewReader(base64.NewDecoder(base64.StdEncoding, bytes.NewBufferString(stat.Data)))
		if err == nil {
			defer gzr.Close()
			stat.data, err = ioutil.ReadAll(gzr)
		}
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				return stat, nil
			}
			return nil, err
		}
	}

	return stat, nil
}

func (fs Fs) MustReadBytes(name string) []byte {
	stat, err := fs.ReadBytes(name)
	if err == nil {
		return stat.data
	}
	return nil
}

/* os.FileInfo */
var _ os.FileInfo = (*File)(nil)

type File struct {
	FilePath    string //文件路径
	FileName    string //文件名
	FileSize    int64  //文件大小
	FileModTime int64  //文件修改时间
	Data        string //压缩编码后的字符串
	FileIsDir   bool   //是否文件夹
	data        []byte //临时存储的数据
}

func (f *File) Name() string {
	return f.FileName
}

func (f *File) Size() int64 {
	return f.FileSize
}

func (f *File) Mode() os.FileMode {
	return 0444
}

func (f *File) ModTime() time.Time {
	return time.Unix(f.FileModTime, 0)
}

func (f *File) IsDir() bool {
	return f.FileIsDir
}

func (f *File) Sys() interface{} {
	return nil
}

/* http.File */
var _ http.File = (*httpFile)(nil)

type httpFile struct {
	stat   *File
	reader io.ReadSeeker //数据读取器
}

func (hf *httpFile) Close() error {
	hf.stat.data = hf.stat.data[:0]
	return nil
}

func (hf *httpFile) Read(p []byte) (n int, err error) {
	if hf.reader != nil {
		return hf.reader.Read(p)
	}
	return -1, io.EOF
}

func (hf *httpFile) Seek(offset int64, whence int) (int64, error) {
	if hf.reader != nil {
		return hf.reader.Seek(offset, whence)
	}
	return -1, io.EOF
}

func (hf *httpFile) Readdir(_ int) ([]os.FileInfo, error) {
	return nil, http.ErrNotSupported
}

func (hf *httpFile) Stat() (os.FileInfo, error) {
	return hf.stat, nil
}
