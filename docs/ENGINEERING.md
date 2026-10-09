# Engineering for long-term ownership

This document explains the development decisions behind the public backend showcase:
how changes are coordinated across services, how routine checks are automated, and how
the code can be understood and maintained by someone other than its original author.
The linked files provide concrete examples of these decisions. Runtime topology,
consistency guarantees and failure handling are covered in [Architecture](ARCHITECTURE.md).

## 1. Make quality standards executable

**Problem.** As a codebase grows, conventions remembered by individuals are difficult to
apply consistently. Reviews can spend time on formatting while missing changes to
behavior or dependencies.

**Decision.** Use formatting and lint rules for repeatable checks, a script for forbidden
package dependencies, and a commit convention, checked by a local hook, for understandable
change history. Local hooks give early feedback; CI re-runs the code checks on the
submitted tree independently of whether a developer installed or ran the hooks. Commit
messages are checked only locally.

**Trade-off.** A strict rule set adds maintenance and can produce noise. Rules should
protect a concrete property, and exceptions should be narrow and explained. Passing a
linter does not establish correctness: reviewers still need to assess business behavior,
concurrency, compatibility and failure modes.

**Evidence:** [lint configuration](../.golangci.yaml),
[architecture rules](../scripts/check-arch.sh), [commit convention](../.commitlintrc.js),
[pre-commit hook](../.husky/pre-commit), [GitHub CI](../.github/workflows/ci.yml).

## 2. Automate repeated work and check generated output

**Problem.** Manual setup commands and repeated edits to derived files make it easy for
local development, generated code and CI to diverge.

**Decision.** Taskfile provides named commands for running services, generation, validation
and tests. Mongo field and collection constants come from models, Wire generates
dependency wiring, and handler annotations produce API documentation. Generated output
is kept in the repository and checked against regeneration. GitLab version variables
have their own generator and drift check.

**Trade-off.** Generators and their inputs become part of the maintenance surface. A change
must include the relevant source and generated output, and drift checks need a tracked
Git baseline. Hooks and CI perform related checks at different scopes: the hook checks
staged-file formatting and newly introduced lint issues, while CI runs the broader gates.
CI also compiles and vets integration-tagged tests that the default lint invocation omits.

**Evidence:** [Taskfile](../Taskfile.yaml), [Mongo generator](../tools/mongo-codegen),
[Wire injector](../services/api/wire/wire.go), [generated API docs](api),
[CI variable generator](../tools/generate-ci-vars.sh).

## 3. Isolate infrastructure decisions behind explicit contracts

**Problem.** Provider details spread quickly when request handlers and business rules call
SDKs directly. That makes changes expensive and tests dependent on external services.

**Decision.** Business services receive repository and broker interfaces through
constructors. Mongo repositories implement the persistence contracts; Kafka and Cloud
Pub/Sub implement the broker abstraction. Wire assembles the service dependency graphs.
Tests can provide in-memory implementations without booting infrastructure.

**Trade-off.** Interfaces reduce the scope of change; they do not make implementations
equivalent. A database replacement still requires decisions about queries, indexes,
transactions, consistency and migration. Broker implementations also have different
delivery and operational behavior. Keep contracts focused on the operations the business
needs, and verify each adapter against the properties it must preserve.

The public repository demonstrates this with broker adapters, Mongo repositories and
test fakes. Email and other provider integrations from the larger project are outside
this extracted slice.

**Evidence:** [repository contracts](../internal/storage/interfaces/repository.go),
[broker package and adapters](../pkg/eventbroker),
[notification publisher](../internal/domains/notification/event_biz.go),
[shared providers](../internal/wire/providers.go), [test fakes](../internal/testutil/fake/fake.go).

## 4. Coordinate related services in a monorepo

**Problem.** The API and worker share models, events and infrastructure conventions.
Splitting those closely related pieces across repositories would add package-release
and synchronization work to changes that need to be reviewed together.

**Decision.** Keep shared code and tooling in one Go module, with separate service entry
points and dependency graphs. Related changes can be reviewed and tested in one commit.
Build targets remain per service; the GitLab pipeline also supports path-filtered builds.

**Trade-off.** A monorepo does not remove coupling or make deployment atomic. Shared-library
changes can affect several services and need broader validation. API and worker versions
may coexist during rollout, so event and data changes still require compatibility planning.
Package dependency checks constrain sharing; they are not a substitute for deciding which
business concepts should be shared. This choice fits the related services in this project,
rather than prescribing one repository for every organization.

**Evidence:** [API entry point](../services/api/main.go),
[worker entry point](../services/worker/main.go), [shared packages](../pkg),
[image build matrix](../.github/workflows/ci.yml), [GitLab pipeline](../.gitlab-ci.yml).

## 5. Make upgrades and failure fixes verifiable

**Problem.** A dependency upgrade or concurrency fix can look correct in review while
breaking transaction behavior, event handling or a deployment assumption.

**Decision.** Centralize the main runtime and tooling versions, and validate behavior at
several levels: domain tests for business rules, real-store integration tests for database
guarantees, and HTTP tests for the assembled application. Regression cases cover rollback,
duplicate events across read entries, concurrent writes and the like/unlike ABA sequence.
Strict integration mode fails when required services or transaction support are missing.

**Trade-off.** Real-service tests cost more than fakes, so each level answers a different
question. HTTP E2E connects publication to the consumer through an in-memory publisher;
it does not establish Kafka transport or rebalance behavior. Replica-count configuration
does not establish throughput. Broker deployment checks and performance measurements need
separate evidence.

For upgrades, review [versions.env](../versions.env) together with [go.mod](../go.mod),
CI action versions and remaining independent tool declarations. Run the quality gates,
unit tests and relevant strict integration tests, then validate the affected deployment
path. Central configuration reduces repetition but does not eliminate compatibility work.

**Evidence:** [repository integration tests](../internal/storage/mongo/repository/integration_test.go),
[HTTP E2E](../services/api/tests/e2e_test.go),
[reconciliation tests](../services/worker/jobs/reconcile_test.go),
[scheduler tests](../services/worker/jobs/scheduler_test.go),
[integration helper](../internal/testutil/mongotest/mongotest.go).

## 6. Leave a maintainable handoff

**Problem.** A successor needs to understand why the system behaves as it does, which
properties must survive a change, and where the current guarantees end.

**Decision.** Keep a runnable post → like → notification slice, a short reviewer tour,
an architecture document, and tests that encode important failure scenarios. Configuration
examples and named Taskfile commands make the entry points discoverable. Document limits
alongside guarantees: bounded event history, best-effort publication, lease overlap and
the standalone database's weaker transaction behavior.

**Trade-off.** Documentation can drift. Changes to contracts, guarantees or operational
steps should update their documentation and tests in the same review. The repository
provides a starting point for onboarding; team ownership, support procedures and production
runbooks still need to reflect the environment in which it is operated.

For a future change, the review questions are:

- Which services, contracts and stored data does it affect?
- Which property must remain true, and what test demonstrates it?
- Can old and new service versions coexist during deployment?
- What happens on partial failure, and how is recovery observed?
- Which generated files, configuration and documentation must change with it?

Start with the [README tour](../README.md#60-second-tour-for-reviewers), follow the
[like use case](../internal/domains/post/biz.go), and use
[Architecture](ARCHITECTURE.md) for the consistency and deployment context.
