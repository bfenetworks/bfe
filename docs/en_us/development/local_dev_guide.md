# Guide of local development

You will learn how to develop BFE in local environment under the guidelines of this document.

## Requirements of coding

- Please refer to the coding format of golang
- Unit test is needed for all codes.
- Pass through all unit tests.
- Please follow [regulations of submmiting codes](submit_pr_guide.md)
  
The following guidiance tells you how to submit code.

## [Fork](https://help.github.com/articles/fork-a-repo/)

Transfer to the home page of Github [BFE](https://github.com/bfenetworks/bfe) ,and then click button `Fork`  to generate the git under your own file directory,such as <https://github.com/USERNAME/bfe>

## Clone

Clone remote git to local:

```bash
$ git clone https://github.com/USERNAME/bfe
$ cd bfe
```

## Create local branch

At present [Git stream branch model](http://nvie.com/posts/a-successful-git-branching-model/)  is applied to BFE to undergo task of development,test,release and maintenance.Please refer to [branch regulation of BFE](release_regulation.md) about details。

All development tasks of feature and bug fix should be finished in a new branch which is extended from `develop` branch.

Create and switch to a new branch with command `git checkout -b`.

```bash
$ git checkout -b my-cool-stuff
```

It is worth noting that before the checkout, you need to keep the current branch directory clean, otherwise the untracked file will be brought to the new branch, which can be viewed by  `git status` .

## Install dependent tools

`make deps` install all the dependent tools, include `pre-commit` `goyacc` `license-eye` `staticcheck`.

### Use `pre-commit` hook

BFE developers use the [pre-commit](http://pre-commit.com/) tool to manage Git pre-commit hooks. It helps us format the source code and automatically check some basic things before committing (such as having only one EOL per file, not adding large files in Git, etc.).

The `pre-commit` test is part of the unit test in Travis-CI. A PR that does not satisfy the hook cannot be submitted to BFE. Install `pre-commit` first and then run it in current directory：

```bash
# ensure installed pre-commit
$ make deps
# enable autoupdate and install hooks
$ make precommit
```

BFE modify the format of golang source code with `gofmt` .

### Use `license-eye` tool

[license-eye](http://github.com/apache/skywalking-eyes) helps us check and fix file's license header declaration. All files' license header should be done before committing.

The `license-eye` check is part of the Github-Action. A PR that check failed cannot be submitted to BFE. Install `license-eye` and do check or fix:

```bash
# ensure installed license-eye
$ make deps
# check the license header
$ make license-check
# fix the license header
$ make license-fix
```

## Start development

I delete a line of README.md and create a new file in the case.

View the current state via `git status` , which will prompt some changes to the current directory, and you can also view the file's specific changes via `git diff` .

```bash
$ git status
On branch test
Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git checkout -- <file>..." to discard changes in working directory)
	modified:   README.md
Untracked files:
  (use "git add <file>..." to include in what will be committed)
	test
no changes added to commit (use "git add" and/or "git commit -a")
```

## Build and test

Please refer to [Build and Run](../installation/install_from_source.md) about construction and test.

## Commit

Next we cancel the modification of README.md,and submit new added test file.

```bash
$ git checkout -- README.md
$ git status
On branch test
Untracked files:
  (use "git add <file>..." to include in what will be committed)
	test
nothing added to commit but untracked files present (use "git add" to track)
$ git add test
```

It's required that the commit message is also given on every Git commit, through which other developers will be notified of what changes have been made. Type `git commit` to realize it.

```bash
$ git commit
CRLF end-lines remover...............................(no files to check)Skipped
yapf.................................................(no files to check)Skipped
Check for added large files..............................................Passed
Check for merge conflicts................................................Passed
Check for broken symlinks................................................Passed
Detect Private Key...................................(no files to check)Skipped
Fix End of Files.....................................(no files to check)Skipped
clang-formatter.......................................(no files to check)Skipped
[my-cool-stuff c703c041] add test file
 1 file changed, 0 insertions(+), 0 deletions(-)
 create mode 100644 233
```

<b> <font color="red">Attention needs to be paid：you need to add commit message to trigger CI test.The command is as follows:</font> </b>

```bash
# Touch CI single test of develop branch
$ git commit -m "test=develop"
# Touch CI single test of release/1.1 branch
$ git commit -m "test=release/1.1"
```

## Keep the latest local repository

It needs to keep up with the latest code of original repository(<https://github.com/bfenetworks/bfe>）before Pull Request.

Check the name of current remote repository with `git remote`.

```bash
$ git remote
origin
$ git remote -v
origin	https://github.com/USERNAME/bfe (fetch)
origin	https://github.com/USERNAME/bfe (push)
```

origin is the name of remote repository that we clone, which is also the BFE under your own account. Next we create a remote host of an original BFE and name it upstream.

```bash
$ git remote add upstream https://github.com/bfenetworks/bfe
$ git remote
origin
upstream
```

Get the latest code of upstream and update current branch.

```bash
$ git fetch upstream
$ git pull upstream develop
```

## Push to remote repository

Push local modification to GitHub(https://github.com/USERNAME/bfe).

```bash
# submit it to remote git the branch my-cool-stuff of origin
$ git push origin my-cool-stuff
```
