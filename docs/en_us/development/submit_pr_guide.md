# Guide of submitting PR to Github

## Create an Issue and finish Pull Request

Create an Issue to describe your problem and keep its number.

Switch to the branch you have created and click `New pull request`.

Switch to targeted branch. A note of `resolve #Issue number` in PR description results in automatic close of corresponding Issue after the merge of PR. More details can be viewed [here](https://help.github.com/articles/closing-issues-via-commit-messages/).

Then please wait for review. If there is any need to make a modification, you can update corresponding branch in origin following the steps above.

## Pass unit tests

Every new commit in your Pull Request will trigger CI unit tests, so please make sure that necessary comments have been included in your commit message. Please refer to [commit](local_dev_guide.md)

Please note the procedure of CI unit tests in your Pull Request which will be finished in several hours.

You only need to focus on CI projects associated with your submitted branch. For example, there is no need to check whether release/1.1 pass test or not if you submit code to develop branch.

Green ticks after all tests means that your commit has passed all unit tests.

Red cross after the tests means your commit hasn't passed certain unit test. Please click detail to view bug details and make a screenshot of bug, then add it as a comment in your Pull Request. Our stuff will help you check it.

## Delete remote branch

We can delete branches of remote repository in PR page after your PR is successfully merged into master repository.

We can also delete the branch of remote repository with `git push origin :the_branch_name`, such as:

```bash
$ git push origin :my-cool-stuff
```

## Delete local branch

Finally,we delete local branch

```bash
$ git checkout develop # switch to develop branch
$ git branch -D my-cool-stuff # delete my-cool-stuff branch

```

And now we finish a full process of code contribution

## Certain regulations about submitting code

In order that reviewers focus on code in the code review, please follow these rules every time you submit your code:

1）Make sure that unit tests in Travis-CI pass through successfully.If it fails,it means problems have been found in submitted code which will not be reviewed by reviewer.

2）Before the submit of PUll Request:

- Please note the number of commit:

Reason：It will bother reviewers a lot if a dozen of commits are submitted after modification of only one file and only a few modifications are updated in every commit. Reviewers have to check commit one by one to figure out the modification. And sometimes it needs to take the overlap among commits into consideration.

Suggestion：Keep commit concise as much as possible at every submit. You can make a supplyment to the previous commit with `git commit --amend`. About several commits having been pushed to remote repository, you can refer to [squash commits after push](http://stackoverflow.com/questions/5667884/how-to-squash-commits-in-git-after-they-have-been-pushed)。

- Pay attention to the name of every commit: It would be better to abstract the content of present commit and be not too arbitrary.

3）If you have tackled with problems of an Issue, please add `fix #issue_number` to the *first* comment area of PULL Request.Then the corresponding Issue will be closed automatically after the merge of PULL Request. Keywords are including:close, closes, closed, fix, fixes, fixed, resolve, resolves, resolved. Please select appropriate word. Please refer to [Closing issues via commit messages](https://help.github.com/articles/closing-issues-via-commit-messages) for more details.

In addition, please follow the following regulations in response to the suggestion of reviewers:

1）A reply to every comment of reviewers（It's a fundamental complimentary conduct in open source community.An expression of appreciation is a need for help from others):

   - If you adopt the suggestion of reviewer and make a modification accordingly, it's courteous to reply with a simple `Done` .

   - Please clarify your reason to the disagreenment

2）If there are many suggestions

   - Please show general modification

   - Please follow [start a review](https://help.github.com/articles/reviewing-proposed-changes-in-a-pull-request/) to give your reply,instead of directly replying for that every comment will result in sending an email causing email disaster.
