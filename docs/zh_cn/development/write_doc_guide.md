# 如何贡献文档

BFE非常欢迎您贡献文档。如果您撰写/翻译的文档满足我们的要求，您的文档将会呈现在bfe-networks.com网站和Github上供BFE的用户阅读。

BFE的文档主要分为以下几个类别：

- 快速入门：旨在帮助用户快速安装和入门；

- 使用指南：旨在为用户提BFE基本用法讲解；

- 开发指南：旨在满足开发者的需求；

我们的文档支持[Markdown](https://guides.github.com/features/mastering-markdown/) (GitHub风格)格式的内容贡献。

撰写文档完成后，您可以使用预览工具查看文档显示的效果，以验证您的文档是否能够在官网正确显示。

## 如何使用预览工具

### 安装依赖项

在此之前，请确认您的操作系统安装了gitbook及依赖项

以ubuntu系统为例，运行：

```bash
$ sudo apt-get update && apt-get install -y npm
$ sudo npm install -g gitbook-cli
```

### Clone仓库

下载完整的BFE仓库：

```bash
$ git clone https://github.com/bfenetworks/bfe
```

### 在本地运行文档站点

进入您希望加载和构建内容的目录列表（docs/LANG）, 运行：

```bash
$ cd docs/zh_cn/
$ gitbook serve --port 8000
...
Serving book on http://localhost:8000
```

然后：打开浏览器并导航到http://localhost:8000。

>*网站可能需要几秒钟才能成功加载，因为构建需要一定的时间*

## 贡献文档

所有内容都应该以[Markdown](https://guides.github.com/features/mastering-markdown/) (GitHub风格)的形式编写。

### 贡献编写文档

- 创建一个新的`.md` 文件或在您当前操作的仓库中修改已存在的文章
- 如果是新增文档，需将新增的文档名，添加到对应的index文件中(SUMMARY.md)

### 运行预览工具

- 在文档基目录(docs/LANG)启动预览工具

```bash
$ cd docs/zh_cn/
$ gitbook serve --port 8000
```

### 预览修改

打开浏览器并导航到http://localhost:8000。

在要更新的页面上，单击右上角的Refresh Content

## 提交修改

修改文档, 提交修改与PR的步骤可以参考[如何贡献代码](../development/local_dev_guide.md)

## 帮助改进预览工具

我们非常欢迎您对平台和支持内容的各个方面做出贡献，以便更好地呈现这些内容。您可以Fork或Clone这个存储库，或者提出问题并提供反馈，以及在issues上提交bug信息。详细内容请参考[开发指南](https://github.com/bfenetworks/bfe/blob/develop/README.md)。
