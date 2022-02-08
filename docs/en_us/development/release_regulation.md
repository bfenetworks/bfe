# BFE Release Regulation

BFE development follows git-flow branching model and [Semantic Versioning](http://semver.org/).

## Branch Regulation

BFE development follows [git-flow](http://nvie.com/posts/a-successful-git-branching-model/), but makes some minor differences for github.

* For the official bfe repository, developers should follow [git-flow](http://nvie.com/posts/a-successful-git-branching-model/).

    * 'master' branch is the stable branch. Latest commit of the 'master' branch is unit-tested and regression-tested.

    * 'develop' branch is the development branch. Every commit of the 'develop' branch is unit-tested, but not regression-tested.

    * 'release/vX.Y.Z' branch is the temporary branch created for release. The code on this branch is undergoing regression testing.

* For the forked bfe repository, developers don't need to strictly abide the git-flow(http://nvie.com/posts/a-successful-git-branching-model/). Each branch of the forked repository is equivalent to feature branch. Specific Suggestions are as follows:

    * Developers synchronize 'develop' branches of the forked repository with that of the official repository.

    * Developers create 'feature' branch from 'develop' branch of the forked repository.

    * After completion of 'feature' branch development, developers submit 'Pull Request' to the official repository for code review.

    * During the review process, developers may continue to modify and submit code in their feature branches.

    * In addition, the 'bugfix' branch is also maintained in the developer's forked repository. Different from the feature branch, developers should submit 'Pull Request' from the 'bugfix' branch to 'master', 'develop' and possibly 'release/vX.Y.Z' branches of the official repository respectively.

## Release Regulation

Follow the following procedures to release a new version:

1. Create a new branch from the 'develop' branch with the name 'release/vX.Y.Z'. For example, `release/v0.10.0`

1. Tag the version of the new branch with 'X.Y.Z-rc.N' (N is patch number). The first tag is'0.10.0-rc.1', the second tag is '0.10.0-rc.2', and so on.

1. For the submission of this version, do the following:

    * Modify version information in 'VERSION' file.

    * Test the functional correctness of the version. If it fails, fixing all the bugs in the 'release/vX.Y.Z' branch, and return to the second step with patch number added by 1.

1. Complete the writing of [Release Note](https://github.com/bfenetworks/bfe/blob/develop/CHANGELOG.md).

1. Merge the 'release/vX.Y.Z' branch into the master branch, and delete the 'release/vX.Y.Z' branch. Merge 'master' branches into the 'develop' branch.

1. Tag the latest commit of the master branch with 'vX.Y.Z'

Note:

* Once a release branch has been created, it is generally not allowed to merge 'release/vX.Y.Z' from the 'develop' branch. This ensures that the 'release/vX.Y.Z' branch is frozen, making it easy for QA to test.

* When the 'release/vX.Y.Z' branch exists, merge the 'bugfix' branch into the 'master', 'develop' and 'release/vX.Y.Z' branches at the same time, if there are bugfix behaviors.
