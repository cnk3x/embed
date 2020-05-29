# goEmbed

> files into a Go executable

## install

```shell script
go get -u -v github.com/shuxs/embed/goEmbed
```

## usage

```shell script
$ goEmbed --help
goEmbed version 1.0.5 build with go1.14.3
Embed files into a Go executable

Usage: goEmbed [OPTIONS] filename/dirname

Options:
      --pkg string          输出的包名
      --name string         输出的变量名称
      --dep string          依赖的资源处理包 (default "github.com/shuxs/embed")
  -o, --out string          输出文件路径
  -f, --force               是否覆盖目标文件
  -m, --match stringArray   筛选(正则表达式)
  -d, --debug               调试模式
  -h, --help                打印使用方法
```

## Other Choice

https://github.com/mjibson/esc
