# BFE Governance

## Principles

The BFE community adheres to the following principles:

- Open: BFE is open source. See [Contributor License Agreement](https://cla-assistant.io/bfenetworks/bfe).
- Welcoming and respectful: See [Code of Conduct](CODE_OF_CONDUCT.md).
- Transparent and accessible: Work and collaboration are done in public.
- Merit: Ideas and contributions are accepted according to their technical merit and alignment with project objectives, scope, and design principles.

## Project Lead

The BFE project has a project lead.

A project lead in BFE is a single person that has a final say in any decision concerning the BFE project.

The term of the project lead is one year, with no term limit restriction.

The project lead is elected by BFE maintainers according to an individual's technical merit to BFE project.

The current project lead is identified in the [MAINTAINERS](MAINTAINERS.md) file`.

## Process for becoming a maintainer

* Express interest to the [project lead](MAINTAINERS.md) that you are interested in becoming a
  maintainer. Becoming a maintainer generally means that you are going to be spending substantial
  time (>20%) on BFE for the foreseeable future. You are expected to have domain expertise and be extremely
  proficient in golang.
* We will expect you to start contributing increasingly complicated PRs, under the guidance
  of the existing senior maintainers.
* We may ask you to do some PRs from our backlog. As you gain experience with the code base and our standards,
  we will ask you to do code reviews for incoming PRs.
* After a period of approximately 3 months of working together and making sure we see eye to eye,
  the existing senior maintainers will confer and decide whether to grant maintainer status or not.
  We make no guarantees on the length of time this will take, but 3 months is an approximate
  goal.

## Maintainer responsibilities

* Classify GitHub issues and perform pull request reviews for other maintainers and the community.

* During GitHub issue classification, apply all applicable [labels](https://github.com/bfenetworks/bfe/labels)
  to each new issue. Labels are extremely useful for follow-up of future issues. Which labels to apply
  is somewhat subjective so just use your best judgment.

* Make sure that ongoing PRs are moving forward at the right pace or closing them if they are not
  moving in a productive direction.

* Participate when called upon in the security release process. Note
  that although this should be a rare occurrence, if a serious vulnerability is found, the process
  may take up to several full days of work to implement.

* In general continue to be willing to spend at least 20% of your time working on BFE (1 day per week).

## When does a maintainer lose maintainer status

* If a maintainer is no longer interested or cannot perform the maintainer duties listed above, they
should volunteer to be moved to emeritus status.

* In extreme cases this can also occur by a vote of the maintainers per the voting process. The voting
process is a simple majority in which each senior maintainer receives two votes and each normal maintainer
receives one vote.

## Changes in Project Lead

Changes in project lead is initiated by opening a github PR.

Anyone from BFE community can vote on the PR with either +1 or -1.

Only the following votes are binding:

1) Any maintainer that has been listed in the [MAINTAINERS](MAINTAINERS.md) file before the PR is opened.
2) Any maintainer from an organization may cast the vote for that organization. However, no organization
should have more binding votes than 1/5 of the total number of maintainers defined in 1).

The PR should only be opened no earlier than 6 weeks before the end of the project lead's term.
The PR should be kept open for no less than 4 weeks. The PR can only be merged after the end of the
last project lead's term, with more +1 than -1 in the binding votes.

When there are conflicting PRs about changes in project lead, the PR with the most binding +1 votes is merged.

The project lead can volunteer to step down.

## Changes in Project Governance

All substantive updates in Governance require a supermajority maintainers vote.

## Decision making process

Decisions are build on consensus between maintainers.
Proposals and ideas can either be submitted for agreement via a github issue or PR,
or by sending an email to `cncf-bfe-maintainers@lists.cncf.io`.

In general, we prefer that technical issues and maintainer membership are amicably worked out between the persons involved.
If a dispute cannot be decided independently, get a third-party maintainer (e.g. a mutual contact with some background
on the issue, but not involved in the conflict) to intercede.
If a dispute still cannot be decided, the project lead has the final say to decide an issue.

Decision making process should be transparent to adhere to the principles of BFE project.

All proposals, ideas, and decisions by maintainers or the project lead
should either be part of a github issue or PR, or be sent to `cncf-bfe-maintainers@lists.cncf.io`.

## Code of Conduct

The [BFE Code of Conduct](CODE_OF_CONDUCT.md) is aligned with the CNCF Code of Conduct.

## Credits

Sections of this documents have been borrowed from [Fluentd](https://github.com/fluent/fluentd/blob/master/GOVERNANCE.md) and [CoreDNS](https://github.com/coredns/coredns/blob/master/GOVERNANCE.md) projects.
