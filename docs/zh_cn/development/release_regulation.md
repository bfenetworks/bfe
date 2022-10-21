# BFE发行规范

BFE使用git-flow branching model做分支管理，使用[Semantic Versioning](http://semver.org/)标准表示BFE版本号。

## 分支规范说明

BFE开发过程使用[git-flow](http://nvie.com/posts/a-successful-git-branching-model/)分支规范，并适应github的特性做了一些区别。

* BFE的主版本库遵循[git-flow](http://nvie.com/posts/a-successful-git-branching-model/)分支规范。其中:
	* `master`分支为稳定(stable branch)版本分支。每一个`master`分支的版本都是经过单元测试和回归测试的版本。
	* `develop`分支为开发(develop branch)版本分支。每一个`develop`分支的版本都经过单元测试，但并没有经过回归测试。
	* `release/vX.Y.Z`分支为每一次Release时建立的临时分支。在这个阶段的代码正在经历回归测试。

* 开发者的fork版本库并不需要严格遵守[git-flow](http://nvie.com/posts/a-successful-git-branching-model/)分支规范，所有fork的版本库的所有分支都相当于特性分支。具体建议如下：
	* 开发者fork的版本库使用`develop`分支同步主版本库的`develop`分支。
	* 开发者fork的版本库中，再基于`develop`版本fork出自己的功能分支。
	* 当功能分支开发完毕后，向BFE的主版本库提交`Pull Request`，进而进行代码评审。
	* 在评审过程中，开发者修改自己的代码，可以继续在自己的功能分支提交代码。
	* 另外，`bugfix`分支也是在开发者自己的fork版本库维护，与功能分支不同的是，`bugfix`分支需要分别给主版本库的`master`、`develop`与可能有的`release/vX.Y.Z`分支，同时提起`Pull Request`。

## 版本发布流程

BFE每次发新的版本，遵循以下流程:

1. 从`develop`分支派生出新的分支，分支名为`release/vX.Y.Z`。例如，`release/v0.10.0`
1. 将新分支的版本打上tag，tag为`vX.Y.Z-rc.N` （N代表Patch号）。第一个tag为`v0.10.0-rc.1`，第二个为`v0.10.0-rc.2`，依次类推。
1. 对这个版本的提交，做如下几个操作:
	* 修改`VERSION`文件中的版本信息。
	* 测试版本的功能正确性。如果失败，在这个`release/vX.Y.Z`分支中修复所有bug，返回第二步并将Patch号加一。
1. 完成[Release Note](https://github.com/bfenetworks/bfe/blob/develop/CHANGELOG.md)的书写。
1. 第三步完成后，将`release/vX.Y.Z`分支合入master分支，并删除`release/vX.Y.Z`分支。同时再将`master`分支合入`develop`分支。
1. 将master分支的合入commit打上tag，tag为`vX.Y.Z`。

需要注意的是:

* `release/vX.Y.Z`分支一旦建立，一般不允许再从`develop`分支合入`release/vX.Y.Z`。这样保证`release/vX.Y.Z`分支功能的封闭，方便测试人员测试BFE的行为。
* 在`release/vX.Y.Z`分支存在的时候，如果有bugfix的行为，需要将bugfix的分支同时merge到`master`, `develop`和`release/vX.Y.Z`这三个分支。
