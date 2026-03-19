Feature: Sigil baseline CLI and config contracts
  The sigil application exposes baseline CLI and configuration behavior
  contracts for initial implementation.

  Background:
    Given a clean sigil working directory
    And SIGIL configuration environment variables are cleared

  Scenario: Defaults application configuration to sigil.yaml and defines baseline app-config schema
    Given the sigil application starts without an explicit application config path
    When application configuration is resolved
    Then the default application config path is "./sigil.yaml"
    And the application config format is "yaml"
    And baseline application config keys are "logs.level" and "logs.dir"

  Scenario: Applies defaults and SIGIL environment overrides for logs.level and logs.dir
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: debug
      """
    And environment override "SIGIL_LOGS_LEVEL" is "warn"
    And environment override "SIGIL_LOGS_DIR" is "./env-logs"
    When application configuration is merged
    Then effective application logs.level is "warn"
    And effective application logs.dir is "./env-logs"

  Scenario: Rejects unsupported logs.level values
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: trace
      """
    When application configuration validation runs
    Then application configuration initialization fails

  Scenario: Exposes the sigil executable entrypoint
    Given the sigil executable is available
    When a user runs `sigil --help`
    Then command exits with status code 0
    And command output contains `sigil`

  Scenario: Prints root usage when sigil is invoked without subcommands
    Given the sigil executable is available
    When a user runs `sigil`
    Then root usage/help is printed
    And command exits with status code 0

  Scenario: Prints run usage when sigil run is invoked without subcommands
    Given the sigil executable is available
    When a user runs `sigil run`
    Then run usage/help is printed
    And command exits with status code 0

  Scenario: Keeps root and run help text when output json is requested
    Given the sigil executable is available
    When a user runs `sigil --output json`
    Then root usage/help is printed
    And command exits with status code 0
    When a user runs `sigil run --output json`
    Then run usage/help is printed
    And command exits with status code 0

  Scenario: Delegates sigil run start behavior to PRD-0130 and PRD-0410 run-start contracts
    Given the sigil executable is available
    And no default start config files exist
    When a user runs `sigil run start`
    Then command exits non-zero
    And command error contains `invalid --config value`

  Scenario: Delegates sigil run stop behavior to PRD-0150 and PRD-0450 run-stop contracts
    Given the sigil executable is available
    When a user runs `sigil run stop`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 0`

  Scenario: Requires exactly one UUIDv7 run-id positional argument for sigil run stop
    Given the sigil executable is available
    When a user runs `sigil run stop`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 0`
    When a user runs `sigil run stop not-a-uuid`
    Then command exits non-zero
    And command error contains `run-id must be UUIDv7`
    When a user runs `sigil run stop 019c7714-3b77-74d1-9866-e1f484aae2ab extra`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 2`

  Scenario: Rejects invalid inherited output value for sigil run start
    Given the sigil executable is available
    When a user runs `sigil run start --output yaml`
    Then command exits non-zero
    And command error contains `output must be one of: text, json`

  Scenario: Rejects invalid inherited output value for sigil run stop
    Given the sigil executable is available
    When a user runs `sigil run stop --output yaml 019c7714-3b77-74d1-9866-e1f484aae2ab`
    Then command exits non-zero
    And command error contains `output must be one of: text, json`

  Scenario: Interrupts an actively running CLI run and prints a human-readable stop result by default
    Given a local CLI run is actively executing
    When a user runs `sigil run stop` for the active run
    Then the active run transitions to "interrupted"
    And command output contains `Run stop result`
    And command output contains `Run ID:`
    And command output contains `Stop requested: true`
    And command output contains `State: interrupted`
    And command output contains `Events path:`

  Scenario: Interrupts an actively running CLI run and prints a terminal JSON stop result when output json is requested
    Given a local CLI run is actively executing
    When a user runs `sigil run stop -o json` for the active run
    Then the active run transitions to "interrupted"
    And stdout contains one JSON stop result with run_id stop_requested state and events_path

  Scenario: Publishes process metadata and persists stop-request metadata for local CLI run control
    Given a local CLI run is actively executing
    When the run lifecycle and stop request metadata are inspected
    Then process.json exists for the active run
    And stop-request.json is written before SIGTERM is issued

  Scenario: Returns terminal no-op JSON stop results for completed failed or interrupted runs when output json is requested
    Given local CLI completed failed and interrupted runs exist
    When terminal stop commands are executed for those runs
    Then each terminal stop command exits with status code 0 and returns stop_requested=false

  Scenario: Waits for terminal state and reports completed or failed JSON results when stop loses the race under output json
    Given stop requests lose the race to completed and failed local CLI runs
    When stop commands are executed for those racing runs
    Then the JSON stop results contain stop_requested=true and the observed terminal states

  Scenario: Fails run stop for unknown runs corrupt event logs or stale process metadata on non-terminal runs
    Given sigil run stop targets unknown corrupt stale and missing-process run state
    When stop commands are executed for those invalid control cases
    Then each invalid control case exits non-zero

  Scenario: Converts startup-window SIGTERM into interrupted terminalization before run.running
    Given a local CLI run has persisted run.queued but not run.running
    When a user runs `sigil run stop` for the active run
    Then the active run transitions to "interrupted"

  Scenario: Persists user-request interruption metadata and partial accounting without synthetic node failure records
    Given a local CLI run is actively executing
    When a user runs `sigil run stop` for the active run
    Then run.interrupted contains reason user_request interrupted_by cli.run.stop and partial accounting
    And interrupted stop handling does not append synthetic node.failed or node.step.completed records

  Scenario: Uses framework-default error behavior for unknown subcommands
    Given the sigil executable is available
    When a user runs `sigil unknown`
    Then command exits non-zero
    And command error contains `unknown command`

  Scenario: Exposes run inspection subcommands under sigil run
    Given the sigil executable is available
    When a user runs `sigil run --help`
    Then command exits with status code 0
    And command output contains `list`
    And command output contains `status`
    And command output contains `inspect`
    And command output contains `events`

  Scenario: Inherits run storage override flag across sigil run subcommands
    Given the sigil executable is available
    When run subcommand help surfaces are inspected for inherited run-dir support
    Then each inspected run help surface documents `--run-dir`

  Scenario: Overrides default run storage base directory with inherited --run-dir for sigil run start
    Given valid application and run configuration inputs
    When a user runs sigil run start with inherited run-dir override in json mode
    Then command exits with status code 0
    And the run start result events path is stored under "./custom-runs"

  Scenario: Rejects explicit empty inherited --run-dir value for sigil run start
    Given the sigil executable is available
    When sigil run start is invoked with an explicit empty inherited run-dir value
    Then command exits non-zero
    And command error contains `invalid --run-dir value; path cannot be empty`

  Scenario: Overrides default run storage base directory with inherited --run-dir for sigil run stop
    Given a terminal run exists only in the selected run directory
    When a user runs sigil run stop for the selected run directory
    Then command exits with status code 0
    And the stop result references the selected run directory "./custom-runs"

  Scenario: Rejects explicit empty inherited --run-dir value for sigil run stop
    Given the sigil executable is available
    When sigil run stop is invoked with an explicit empty inherited run-dir value
    Then command exits non-zero
    And command error contains `invalid --run-dir value; path cannot be empty`

  Scenario: Lists runs newest-first from the selected run directory
    Given one or more persisted runs exist in the selected run directory
    When a user runs sigil run list for the selected run directory
    Then the returned run summaries are ordered newest-first by queued time

  Scenario: Returns empty success when sigil run list targets a missing selected run directory
    Given the selected run directory does not exist
    When a user runs sigil run list for the missing selected run directory
    Then the command exits with status code 0 and returns an empty result

  Scenario: Requires exactly one UUIDv7 run-id positional argument for sigil run status
    Given the sigil executable is available
    When a user runs `sigil run status`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 0`
    When a user runs `sigil run status not-a-uuid`
    Then command exits non-zero
    And command error contains `run-id must be UUIDv7`
    When a user runs `sigil run status 019c7714-3b77-74d1-9866-e1f484aae2ab extra`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 2`

  Scenario: Requires exactly one UUIDv7 run-id positional argument for sigil run inspect
    Given the sigil executable is available
    When a user runs `sigil run inspect`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 0`
    When a user runs `sigil run inspect not-a-uuid`
    Then command exits non-zero
    And command error contains `run-id must be UUIDv7`
    When a user runs `sigil run inspect 019c7714-3b77-74d1-9866-e1f484aae2ab extra`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 2`

  Scenario: Requires exactly one UUIDv7 run-id positional argument for sigil run events
    Given the sigil executable is available
    When a user runs `sigil run events`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 0`
    When a user runs `sigil run events not-a-uuid`
    Then command exits non-zero
    And command error contains `run-id must be UUIDv7`
    When a user runs `sigil run events 019c7714-3b77-74d1-9866-e1f484aae2ab extra`
    Then command exits non-zero
    And command error contains `accepts 1 arg(s), received 2`

  Scenario: Rejects invalid inherited output value for sigil run inspection commands
    Given the sigil executable is available
    When a user runs `sigil run list --output yaml`
    Then command exits non-zero
    And command error contains `output must be one of: text, json`

  Scenario: Prints run list summaries in text and json output modes
    Given persisted runs exist in the selected run directory
    When a user runs sigil run list in text mode and in json mode
    Then both outputs return the same run-summary set in their respective formats

  Scenario: Prints run status summary in text and json output modes
    Given a targeted persisted run exists in the selected run directory
    When a user runs sigil run status for the targeted run in text mode and in json mode
    Then both outputs return the same run summary in their respective formats

  Scenario: Prints run inspection summary in text and json output modes
    Given a targeted persisted run exists in the selected run directory
    When a user runs sigil run inspect for the targeted run in text mode and in json mode
    Then both outputs return the same run inspection summary in their respective formats

  Scenario: Returns canonical run events in append order for sigil run events
    Given a targeted persisted run exists in the selected run directory
    When a user runs sigil run events for the targeted run in json mode
    Then the returned event stream preserves canonical append order

  Scenario: Uses inherited run-dir override across sigil run inspection commands
    Given persisted runs exist outside the default run storage directory
    When run inspection commands are executed with inherited run-dir override
    Then only runs stored under "./custom-runs" are inspected

  Scenario: Derives run summary from canonical events and auxiliary control metadata
    Given canonical events and auxiliary control metadata exist for one run
    When a run summary is requested from the selected run directory
    Then the summary is derived from canonical events plus auxiliary control metadata

  Scenario: Derives run projection from canonical events without materializing a second source of truth
    Given canonical events and run-local refs exist for one run
    When a run projection is requested from the selected run directory
    Then the projection is derived on demand without persisting a separate read model

  Scenario: Reports pid status as current missing not_running or stale from process metadata
    Given process metadata states vary across runs
    When run summaries are derived from the selected run directory
    Then pid_status reports current missing not_running or stale accordingly

  Scenario: Surfaces stop_requested from stop-request metadata without changing event authority
    Given stop-request metadata exists for one run
    When a run summary or run projection is requested from the selected run directory
    Then stop_requested is surfaced without changing canonical event authority

  Scenario: Preserves final answer and accounting as refs in run projection output
    Given canonical terminal refs exist for one run
    When a run projection is requested from the selected run directory
    Then final-answer and accounting data are exposed as refs rather than inline artifact bodies

  Scenario: Fails targeted run queries when canonical event logs are missing or corrupt
    Given a targeted run has missing or corrupt canonical event storage
    When targeted run queries are requested from the selected run directory
    Then targeted run queries fail for missing or corrupt canonical event storage

  Scenario: Loads default run configuration file from current working directory
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: default prompt
      context: default context
      llm:
        provider: openai
        model: gpt-5.1
      """
    And the sigil application starts without an explicit run config path
    When run configuration is merged
    Then the default run config path is "./sigil-run.yaml"
    And run configuration initialization succeeds

  Scenario: Applies SIGIL_RUN environment variable overrides using dot-to-underscore key mapping
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: file prompt
      context: file context
      llm:
        provider: anthropic
        model: claude-sonnet-4
      """
    And environment override "SIGIL_RUN_LLM_PROVIDER" is "openai"
    And environment override "SIGIL_RUN_LLM_MODEL" is "gpt-5.1"
    When run configuration is merged
    Then effective run llm.provider is "openai"
    And effective run llm.model is "gpt-5.1"

  Scenario: Rejects run configuration when llm.provider or llm.model is missing after merge
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Requires exactly one of prompt and prompt_template
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      prompt_template: prompt-template
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Requires exactly one of context and context_template
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      context_template: context-template
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Defaults llm.gateway to openrouter when omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run llm.gateway is "openrouter"

  Scenario: Allows omitted llm.openrouter block when gateway is openrouter
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run llm.openrouter.base_url is "https://openrouter.ai/api/v1"
    And effective run llm.openrouter.request_timeout_ms is 30000
    And effective run llm.openrouter.api_key_env is "OPENROUTER_API_KEY"

  Scenario: Rejects unsupported llm.gateway values
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        gateway: invalid-gateway
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Applies OpenRouter defaults when openrouter fields are omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        openrouter: {}
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run llm.openrouter.base_url is "https://openrouter.ai/api/v1"
    And effective run llm.openrouter.request_timeout_ms is 30000
    And effective run llm.openrouter.api_key_env is "OPENROUTER_API_KEY"

  Scenario: Applies RLM defaults when rlm fields are omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run rlm.enabled is true
    And effective run rlm.max_depth is 3

  Scenario: Accepts required values provided entirely by environment variables
    Given no default run config files exist
    And environment override "SIGIL_RUN_NAME" is "test-run"
    And environment override "SIGIL_RUN_PROMPT" is "env prompt"
    And environment override "SIGIL_RUN_CONTEXT" is "env context"
    And environment override "SIGIL_RUN_LLM_PROVIDER" is "openai"
    And environment override "SIGIL_RUN_LLM_MODEL" is "gpt-5.1"
    When run configuration is resolved
    Then run configuration initialization succeeds
    And effective run llm.provider is "openai"
    And effective run llm.model is "gpt-5.1"

  Scenario: Accepts llm.provider only when value is openai or anthropic
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: anthropic
        model: claude-sonnet-4
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: unsupported
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Validates llm.model against allowed list for provider openai
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: claude-sonnet-4
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Validates llm.model against allowed list for provider anthropic
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: anthropic
        model: claude-sonnet-4
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: anthropic
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Rejects run configuration when llm.model is not allowed for selected llm.provider
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: claude-sonnet-4
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Applies default reasoning config values when llm.reasoning block is omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run llm.reasoning.enabled is true
    And effective run llm.reasoning.effort is "medium"

  Scenario: Accepts llm.reasoning.effort only when value is minimal low medium or high
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        reasoning:
          effort: minimal
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        reasoning:
          effort: low
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        reasoning:
          effort: high
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        reasoning:
          effort: extreme
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Allows reasoning effort to be present and ignored when llm.reasoning.enabled is false
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        reasoning:
          enabled: false
          effort: high
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run llm.reasoning.enabled is false
    And effective run llm.reasoning.effort is "high"

  Scenario: Applies SIGIL_RUN environment overrides for llm.reasoning.enabled and llm.reasoning.effort
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
        reasoning:
          enabled: true
          effort: low
      """
    And environment override "SIGIL_RUN_LLM_REASONING_ENABLED" is "false"
    And environment override "SIGIL_RUN_LLM_REASONING_EFFORT" is "high"
    When run configuration is merged
    Then effective run llm.reasoning.enabled is false
    And effective run llm.reasoning.effort is "high"

  Scenario: Uses default application and run configuration paths when no flags are provided
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start`
    Then command exits with status code 0

  Scenario: Overrides application configuration path with --config
    Given application config exists at "./custom-sigil.yaml" with:
      """
      logs:
        level: warn
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start --config ./custom-sigil.yaml`
    Then command exits with status code 0

  Scenario: Overrides run configuration path with --run-config
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./custom-run.yaml"
    When a user runs `sigil run start --run-config ./custom-run.yaml`
    Then command exits with status code 0

  Scenario: Overrides both configuration paths when both flags are provided
    Given application config exists at "./custom-sigil.yaml" with:
      """
      logs:
        level: debug
      """
    And run config file exists at "./custom-run.yaml"
    When a user runs `sigil run start --config ./custom-sigil.yaml --run-config ./custom-run.yaml`
    Then command exits with status code 0

  Scenario: Fails when required configuration paths are missing unreadable or not regular files
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And a directory exists at "./not-a-file"
    When a user runs `sigil run start --config ./not-a-file`
    Then command exits non-zero
    And command error contains `invalid --config value`
    When a user runs `sigil run start --run-config ./missing-run.yaml`
    Then command exits non-zero
    And command error contains `invalid --run-config value`
    When a user runs `sigil run start --unknown-flag`
    Then command exits non-zero
    And command error contains `unknown flag`

  Scenario: Accepts repeated --var key value entries
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt_template: prompt {{.name}}
      context_template: context {{.name}}
      llm:
        provider: openai
        model: gpt-5.1
      """
    When a user runs `sigil run start --var name=acme --var environment=prod`
    Then command exits with status code 0

  Scenario: Rejects invalid --var format or empty key
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start --var invalid`
    Then command exits non-zero
    And command error contains `invalid --var value`

  Scenario: Resolves duplicate --var keys deterministically using last value
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt_template: prompt {{.name}}
      context_template: context {{.name}}
      llm:
        provider: openai
        model: gpt-5.1
      """
    When a user runs `sigil run start --var name=first --var name=second`
    Then command exits with status code 0

  Scenario: Executes sigil run start as a blocking harness entrypoint
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start`
    Then command exits with status code 0
    And command output contains `Run summary`

  Scenario: Emits run.queued with source cli.run.start and resolved config-path metadata
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start`
    Then command exits with status code 0
    And command output contains `Run queued: run_id=`
    And command output contains `Run running: run_id=`

  Scenario: Starts lifecycle and root node before first inference step
    Given a run in "queued" state
    When harness execution starts
    Then run transitions to "running"
    And exactly one root node exists with depth 0 and null parent

  Scenario: Resolves effective prompt and context from direct fields or rendered templates
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt_template: prompt {{.name}}
      context_template: context {{.name}}
      llm:
        provider: openai
        model: gpt-5.1
      """
    When a user runs `sigil run start --var name=acme`
    Then command exits with status code 0

  Scenario: Fails run start on template rendering errors under strict missing-key policy
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt_template: prompt {{.missing}}
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When a user runs `sigil run start`
    Then command exits non-zero
    And command error contains `harness_template_render`

  Scenario: Executes multi-step decision loop until root terminal decision
    Given an active node with existing REPL session state
    When additional continue actions execute for that node
    Then subsequent actions run in the same node REPL session state

  Scenario: Persists per-step user and model turns and emits node turn events
    Given a node-local step is in progress
    When transcript contributions are persisted for that step
    Then each turn contribution is recorded with role user or model

  Scenario: Executes exactly one continue action per continue decision and emits node.action.executed
    Given an active node with continuation payload containing non-empty continuation.repl_code
    When harness processes continuation step
    Then exactly one continuation.repl_code action executes in node-local REPL state

  Scenario: Allows recursive child-node execution via rlm_query when recursion is enabled and depth permits
    Given an active parent node at depth 1
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then child node is created at depth 2

  Scenario: Falls back to plain subcall behavior when recursion exceeds rlm.max_depth in recursive mode
    Given an active parent node at depth 3
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then plain subcall fallback answer is returned and child node is not created

  Scenario: Runs non-recursive multi-step profile when rlm.enabled is false
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      rlm:
        enabled: false
      """
    When a user runs `sigil run start`
    Then command exits with status code 0
    And command output contains `Profile: non-recursive`
    And command output contains `State: completed`

  Scenario: Returns typed depth-limit error for all rlm_query calls in non-recursive mode
    Given non-recursive harness mode is active
    When rlm_query is invoked from node-local Go REPL context
    Then typed depth-limit error is returned and child node is not created

  Scenario: Completes run on root final answer and sets run.completed.final_answer_ref
    Given an active root node inference result is decision final with answer "final answer"
    When harness evaluates root node step
    Then run transitions to "completed"
    And run completion references terminal root final output

  Scenario: Prints human-readable run progress and terminal summary by default
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start`
    Then command exits with status code 0
    And command output contains `Run start`
    And command output contains `Run queued: run_id=`
    And command output contains `Run running: run_id=`
    And command output contains `Run summary`
    And command output contains `State: completed`
    And command output contains `Events path:`
    And command output contains `Final answer ref:`
    And command output contains `Final answer:`
    And command output contains `Accounting:`

  Scenario: Prints JSON run summary on successful completion when output json is requested
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start -o json`
    Then command exits with status code 0
    And command output contains `"run_id"`
    And command output contains `"final_answer_ref"`
    And command output contains `"events_path"`
    And command output contains `"final_answer"`
    And command output contains `"accounting"`

  Scenario: Shows recursive child-node growth in text output
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: root prompt
      context: root context
      llm:
        provider: openai
        model: gpt-5.1
      rlm:
        enabled: true
        max_depth: 2
      """
    And CLI run start mock responses are configured for "recursive-progress"
    When a user runs `sigil run start`
    Then command exits with status code 0
    And command output contains `Profile: recursive`
    And command output contains `role=recursive_subcall parent_node_id=`
    And command output contains `mode=recursive`

  Scenario: Shows fallback subcall mode in text output when recursion depth is exceeded
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: root prompt
      context: root context
      llm:
        provider: openai
        model: gpt-5.1
      rlm:
        enabled: true
        max_depth: 0
      """
    And CLI run start mock responses are configured for "fallback-progress"
    When a user runs `sigil run start`
    Then command exits with status code 0
    And command output contains `Profile: recursive`
    And command output contains `mode=fallback`

  Scenario: Exits non-zero with typed failure metadata on unrecoverable harness inference or template errors
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
      """
    And run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt_template: prompt {{.missing}}
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When a user runs `sigil run start`
    Then command exits non-zero
    And command error contains `harness_template_render`

  Scenario: Writes application logs to a derived sigil.log file path
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
        dir: ./derived-log-dir
      """
    When application logging is initialized
    Then the effective log file path is "./derived-log-dir/sigil.log"

  Scenario: Uses JSON structured log records for application logging
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
        dir: ./json-log-dir
      """
    When application logging is initialized
    And application logging writes an info record with message "json-acceptance-record"
    Then log records are structured JSON

  Scenario: Uses default log file path when default logs.dir is in effect
    Given the sigil application starts without an explicit application config path
    When application logging is initialized
    Then the effective log target path is "./.sigil/logs/sigil.log"

  Scenario: Fails initialization when derived log file path cannot be opened as a file sink
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
        dir: ./blocked-log-target
      """
    And a file exists at "./blocked-log-target"
    When a user runs `sigil run`
    Then command exits non-zero
    And command error contains `failed to initialize application logging`

  Scenario: Derives log file path from configured logs.dir override
    Given application config exists at "./sigil.yaml" with:
      """
      logs:
        level: info
        dir: ./override-log-dir
      """
    When application logging is initialized
    Then the effective log target path is "./override-log-dir/sigil.log"

  Scenario: Initializes runs in queued state before execution begins
    Given a new run is created
    When lifecycle initialization completes
    Then run state is "queued"

  Scenario: Transitions run from queued to running when execution starts
    Given a run in "queued" state
    When execution starts
    Then run transitions to "running"

  Scenario: Creates exactly one root node at depth zero for each run
    Given a run transitions to "running"
    When node initialization occurs
    Then exactly one root node exists with depth=0 and parent_node_id=null

  Scenario: Allows child nodes only under an existing parent node in the same run
    Given a run in "running" state
    When a child node is created
    Then it references an existing parent node in the same run

  Scenario: Transitions run to completed on successful execution termination
    Given a run in "running" state
    When execution terminates successfully
    Then run transitions to "completed"

  Scenario: Transitions run to failed on unrecoverable runtime failure
    Given a run in "running" state
    When unrecoverable runtime failure occurs
    Then run transitions to "failed"

  Scenario: Transitions run to interrupted on explicit interruption
    Given a run in "running" state
    When explicit interruption is requested
    Then run transitions to "interrupted"

  Scenario: Transitions run from queued to interrupted when explicit interruption arrives before execution begins
    Given a run in "queued" state
    When explicit interruption is requested
    Then run transitions to "interrupted"

  Scenario: Rejects invalid transitions from terminal run states
    Given a run in "running" state
    When execution terminates successfully
    And any further state transition is requested
    Then transition validation fails
    Given a run in "running" state
    When unrecoverable runtime failure occurs
    And any further state transition is requested
    Then transition validation fails
    Given a run in "running" state
    When explicit interruption is requested
    And any further state transition is requested
    Then transition validation fails

  Scenario: Represents tool and code execution as node-scoped events without creating nodes
    Given a run in "running" state with active recursive nodes
    When tool or code execution activity occurs
    Then activity is recorded as node-scoped events and no additional node entity is created

  Scenario: Persists run events to per-run append-only events.jsonl under .sigil runs directory
    Given a persisted lifecycle run exists
    When canonical run lifecycle events are emitted
    Then events are persisted to a per-run append-only events.jsonl path under .sigil runs directory

  Scenario: Uses UUIDv7 identifiers for run node and event identity fields
    Given persisted canonical run events exist
    When persisted event identity fields are inspected
    Then run_id node_id when present and event_id are UUIDv7

  Scenario: Assigns contiguous per-run sequence numbers starting at one
    Given persisted canonical run events exist
    When persisted event sequence values are inspected
    Then seq starts at 1 and increments contiguously by 1

  Scenario: Writes one valid JSON event envelope per line
    Given persisted canonical run events exist
    When events.jsonl is parsed line by line
    Then each non-empty line is a valid JSON event envelope

  Scenario: Requires run_id on all events and node_id on node-scoped events
    Given persisted canonical run events exist
    When required identity fields are validated
    Then all events contain run_id and node-scoped events contain node_id

  Scenario: Fsyncs each appended event before acknowledging persistence
    Given persisted canonical run events exist
    When persistence acknowledgement metrics are inspected
    Then each appended event has been fsynced before acknowledgement

  Scenario: Rejects event append when next sequence is not contiguous
    Given persisted canonical run events exist
    When an event append is requested with non-contiguous next sequence
    Then event append is rejected for non-contiguous sequence

  Scenario: Fails integrity validation on malformed or partial persisted event lines
    Given persisted canonical run events exist
    And events.jsonl is corrupted with malformed or partial lines
    When event-log integrity validation executes
    Then integrity validation fails for run recovery

  Scenario: Preserves immutability by forbidding in-place event modification
    Given persisted canonical run events exist
    When an append attempts in-place sequence rewrite
    Then event append is rejected by immutable event-store contract

  Scenario: Carries schema_version to support forward event evolution
    Given persisted canonical run events exist
    When event envelopes are inspected
    Then schema_version exists and equals v1

  Scenario: Defines canonical runtime event type catalog for v1
    Given canonical v1 run-event validation rules
    When canonical runtime event types are validated
    Then only canonical v1 runtime event types are accepted

  Scenario: Enforces strict payload schema and invariants for each canonical v1 event type
    Given canonical v1 events with payloads
    When strict payload schema validation is executed
    Then required fields types and invariants are enforced per event type

  Scenario: Rejects unknown fields and unknown event types under v1 strict extensibility rules
    Given v1 event envelopes with unknown fields or unknown type
    When strict v1 extensibility validation is executed
    Then validation fails and events are rejected

  Scenario: Defers non-core tool and model payload families while keeping canonical event payloads normative
    Given a canonical v1 event payload includes deferred non-core fields
    When canonical v1 payload validation executes
    Then deferred non-core fields are rejected as out-of-contract

  Scenario: Defines node step event types for decision-cycle tracking
    Given canonical v1 run-event validation rules
    When event type validation includes node step tracking events
    Then node.step.started and node.step.completed are accepted canonical event types

  Scenario: Enforces strict payload schema and invariants for node step events
    Given canonical node step events with payloads
    When strict payload schema validation is executed
    Then required node step fields and decision-action invariants are enforced

  Scenario: Defines node turn event types for user and model transcript contributions
    Given canonical v1 run-event validation rules
    When event type validation includes node turn events
    Then node.turn.user and node.turn.model are accepted canonical event types

  Scenario: Enforces strict payload schema and role invariants for node turn events
    Given canonical node turn events with payloads
    When strict payload schema validation is executed
    Then required node turn fields are enforced and role values match event type semantics

  Scenario: Defines and validates node.action.executed payloads for single-action continue steps
    Given canonical node action execution events with payloads
    When strict payload schema validation is executed
    Then node.action.executed payload enforces single-action continue invariants

  Scenario: Defines node.subcall.executed event type for subcall observability
    Given canonical v1 run-event validation rules
    When canonical runtime event types are validated
    Then only canonical v1 runtime event types are accepted

  Scenario: Enforces strict payload schema and invariants for node.subcall.executed events
    Given canonical v1 events with payloads
    When strict payload schema validation is executed
    Then required fields types and invariants are enforced per event type

  Scenario: Resolves inference gateway through registry using llm.gateway
    Given a valid inference request for gateway resolution
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference gateway resolution runs
    Then resolution occurs through gateway registry lookup

  Scenario: Uses OpenRouter Responses API in non-streaming mode for v1
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then request targets OpenRouter Responses API in non-streaming mode

  Scenario: Resolves schema_id sigil.rlm.response.v1 from central registry for inference requests
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then schema is resolved from central registry and applied to request

  Scenario: Uses schema_id sigil.rlm.response.v1 for terminal inference responses
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then schema is resolved from central registry and applied to request

  Scenario: Requires strict json_schema structured outputs on all inference requests
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then strict json_schema structured output mode is required

  Scenario: Enables response healing plugin on all inference requests
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then response healing plugin is enabled

  Scenario: Applies reasoning configuration when llm.reasoning.enabled is true
    Given a valid inference request for execution
    And inference reasoning is enabled with effort "medium"
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then reasoning config is included using configured effort "medium"

  Scenario: Omits reasoning request block when llm.reasoning.enabled is false
    Given a valid inference request for execution
    And inference reasoning is disabled
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then reasoning config is omitted

  Scenario: Retries up to three total attempts with exponential backoff on 429 and 5xx responses
    Given a valid inference request for execution
    And openrouter mock gateway returns retry status sequence "429,500,200"
    When inference execution runs
    Then runtime retries with bounded policy (3 total attempts exponential backoff base 250ms jitter max 2s)

  Scenario: Fails with typed inference error after bounded retries are exhausted
    Given a valid inference request for execution
    And openrouter mock gateway returns retry status sequence "429,500,503"
    When inference execution runs
    Then inference fails with typed error code "gateway_failure"

  Scenario: Fails with typed inference error when healed response does not satisfy strict schema
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "schema-invalid"
    When inference execution runs
    Then inference fails with typed error code "output_validation"

  Scenario: Fails with typed inference error when configured provider model pair does not support required reasoning behavior
    Given a valid inference request for execution
    And inference reasoning is enabled with effort "medium"
    And openrouter mock gateway returns payload fixture "reasoning-unsupported"
    When inference execution runs
    Then inference fails with typed error code "reasoning_capability"

  Scenario: Rejects inference request when schema_id is not found in central registry
    Given a valid inference request for execution
    And central inference schema registry excludes schema_id "sigil.unknown.v1"
    When inference execution runs
    Then inference fails with typed error code "schema_lookup"

  Scenario: Returns canonical normalized inference response shape on success
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Requires decision discriminator values continue or final in sigil.rlm.response.v1
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "decision-invalid"
    When inference execution runs
    Then decision discriminator enforces continue or final

  Scenario: Requires continuation repl_code and forbids final branch when decision is continue
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "continue-branch-invalid"
    When inference execution runs
    Then continuation branch invariant is enforced

  Scenario: Requires final branch and forbids continuation branch when decision is final
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "final-branch-invalid"
    When inference execution runs
    Then final branch invariant is enforced

  Scenario: Rejects unknown fields in sigil.rlm.response.v1 payloads
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "unknown-field"
    When inference execution runs
    Then unknown fields are rejected with typed output-validation error

  Scenario: Emits reasoning data under top-level reasoning key and reasoning token counts under usage
    Given a valid inference request for execution
    And inference reasoning is enabled with effort "medium"
    And openrouter mock gateway returns payload fixture "reasoning-artifacts"
    When inference execution runs
    Then reasoning artifacts are under top-level reasoning and reasoning token counts are under usage.reasoning_tokens

  Scenario: Resolves schema_id sigil.llm.answer.v1 from central registry for plain subcall inference requests
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-valid"
    When inference request construction runs
    Then schema is resolved from central registry and applied to request

  Scenario: Requires non-empty answer field in sigil.llm.answer.v1 payloads
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-empty"
    When inference execution runs
    Then inference fails with typed error code "output_validation"

  Scenario: Constructs plain-subcall inference requests as ordered message arrays
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-valid"
    When inference request construction runs
    Then request uses message-array input preserving role order

  Scenario: Supports cheap plain-subcall path with reasoning omitted from request payload
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And inference reasoning is disabled
    And openrouter mock gateway returns payload fixture "llm-answer-valid"
    When inference request construction runs
    Then reasoning config is omitted

  Scenario: Resolves OpenAI base system prompt when llm.provider is openai
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When harness base system prompt resolution runs
    Then resolved base system prompt is "openai"

  Scenario: Resolves Anthropic base system prompt when llm.provider is anthropic
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: anthropic
        model: claude-sonnet-4
      """
    When harness base system prompt resolution runs
    Then resolved base system prompt is "anthropic"

  Scenario: Falls back to OpenAI base system prompt when provider-specific prompt is not registered
    Given unregistered provider key "provider-x" for system prompt resolution
    When harness base system prompt resolution runs
    Then resolved base system prompt is "openai"

  Scenario: Appends system_prompt_append to resolved base system prompt when append is non-empty
    Given resolved base system prompt is "openai"
    And run config system_prompt_append is "Use concise tool calls."
    When harness effective system prompt is constructed
    Then effective system prompt equals base prompt plus two newlines plus append text

  Scenario: Uses resolved base system prompt unchanged when system_prompt_append is empty
    Given resolved base system prompt is "anthropic"
    And run config system_prompt_append is ""
    When harness effective system prompt is constructed
    Then effective system prompt equals resolved base prompt

  Scenario: Uses block-structured OpenAI system prompt with hard finalization gate
    Given resolved base system prompt is "openai"
    When harness effective system prompt is constructed
    Then openai system prompt uses block sections and hard finalization gate

  Scenario: Includes OpenAI search discipline and timeout recovery guidance
    Given resolved base system prompt is "openai"
    When harness effective system prompt is constructed
    Then openai system prompt includes search discipline and timeout recovery rules

  Scenario: Explains the plain subcall answer-string contract in the OpenAI prompt
    Given resolved base system prompt is "openai"
    When harness effective system prompt is constructed
    Then openai system prompt explains the plain subcall answer-string contract

  Scenario: Keeps structured subcall prompt examples compile-safe in the OpenAI prompt
    Given resolved base system prompt is "openai"
    When harness effective system prompt is constructed
    Then openai system prompt explains compile-safe structured prompt strings

  Scenario: Uses safe structured parsing guidance in the OpenAI prompt
    Given resolved base system prompt is "openai"
    When harness effective system prompt is constructed
    Then openai system prompt explains safe structured parsing in repl code

  Scenario: Keeps Anthropic prompt simpler while preserving safety-critical evidence rules
    Given resolved base system prompt is "anthropic"
    When harness effective system prompt is constructed
    Then anthropic system prompt preserves safety rules without openai block sections

  Scenario: Starts harness execution with one root node at depth zero
    Given a run in "queued" state
    When harness execution starts
    Then run transitions to "running"
    And exactly one root node exists with depth 0 and null parent

  Scenario: Executes single continuation repl_code action from structured response in node-local REPL state
    Given an active node with continuation payload containing non-empty continuation.repl_code
    When harness processes continuation step
    Then exactly one continuation.repl_code action executes in node-local REPL state

  Scenario: Propagates typed output-validation error when continue payload omits continuation repl_code
    Given an active node continuation payload with decision continue and missing continuation.repl_code
    When strict output validation runs
    Then continuation step fails with typed output-validation error

  Scenario: Creates child node through rlm_query only when resulting depth does not exceed rlm.max_depth
    Given an active parent node at depth 1
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then child node is created at depth 2

  Scenario: Falls back to plain llm_query behavior when rlm_query call would exceed rlm.max_depth in recursive mode
    Given an active parent node at depth 3
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then plain subcall fallback answer is returned and child node is not created

  Scenario: Returns child final answer to caller REPL context on successful recursive subcall
    Given an active parent node with child node in progress
    And child node inference result is decision final with answer "child answer"
    When child node completes
    Then caller REPL context receives rlm_query result "child answer"

  Scenario: Completes run when root node emits decision final with non-empty final answer
    Given an active root node inference result is decision final with answer "final answer"
    When harness evaluates root node step
    Then run transitions to "completed"
    And run completion references terminal root final output

  Scenario: Defines step as one node-local decision cycle
    Given an active node in harness execution
    When one inference request and response handling cycle complete
    Then exactly one node-local step is recorded

  Scenario: Records turns as user or model transcript contributions linked to a step
    Given a node-local step is in progress
    When transcript contributions are persisted for that step
    Then each turn contribution is recorded with role user or model

  Scenario: Limits continue steps to exactly one executable action
    Given a node-local step with decision continue
    When continuation payload is validated
    Then exactly one executable action is accepted for that step

  Scenario: Runs non-recursive multi-step profile when rlm.enabled is false and returns typed depth-limit feedback for recursive subcalls
    Given non-recursive harness mode is active
    When rlm_query is invoked from node-local Go REPL context
    Then typed depth-limit error is returned and child node is not created

  Scenario: Selects embedded Go REPL engine architecture for v1
    Given v1 REPL runtime architecture rules
    When REPL engine configuration is resolved
    Then embedded in-process Go interpretation is selected

  Scenario: Creates exactly one persistent REPL session per node
    Given an active node with no existing REPL session
    When first continue action executes
    Then one REPL session is created and associated to that node

  Scenario: Reuses node REPL state across multiple continue steps
    Given an active node with existing REPL session state
    When additional continue actions execute for that node
    Then subsequent actions run in the same node REPL session state

  Scenario: Closes node REPL session on node completion and run termination
    Given active node REPL sessions exist
    When node completes or run enters terminal state
    Then corresponding REPL sessions are closed

  Scenario: Executes continuation.repl_code in node-local REPL session
    Given a continue step with non-empty continuation.repl_code
    When harness executes action handling
    Then continuation.repl_code executes in the current node-local REPL session

  Scenario: Exposes llm_query rlm_query llm_query_batched and rlm_query_batched in node-local REPL session
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Creates child node for rlm_query when depth is within limit
    Given parent node depth and run max recursion depth permit recursion
    When rlm_query is invoked from node-local Go REPL context
    Then child node is created and executed

  Scenario: Falls back to llm_query when rlm_query reaches max depth in recursive mode
    Given an active parent node at depth 3
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then plain subcall fallback answer is returned and child node is not created

  Scenario: Falls back to llm_query after a small-context node already used recursive subcalls in a prior step
    Given a small-context harness runner already used recursive subcalls in a prior continue step
    When the next step invokes rlm_query on that same node
    Then the next-step execution state disables recursive subcalls
    And repeated small-context rlm_query uses plain fallback without creating another child node

  Scenario: Returns child final answer to caller REPL context on successful subcall
    Given an active parent node with child node in progress
    And child node inference result is decision final with answer "child answer"
    When child node completes
    Then child final answer is returned to caller REPL context

  Scenario: Records failed action and continues next step on non-fatal REPL execution error
    Given a continue action fails with non-fatal REPL execution error
    When action failure is handled
    Then action failure is recorded and node execution continues to next step

  Scenario: Enforces 180-second execution timeout per action
    Given a continue action exceeding 180 seconds execution time
    When REPL runtime enforces guardrails
    Then action times out with typed timeout error

  Scenario: Rejects repl_code payloads larger than 65536 bytes
    Given a continue action with repl_code payload larger than 65536 bytes
    When payload guardrails are validated
    Then action is rejected with typed code-size error

  Scenario: Truncates stdout/stderr deterministically at 1048576-byte caps
    Given an action execution producing stdout or stderr over 1048576 bytes
    When output capture guardrails are enforced
    Then outputs are truncated with deterministic truncation marker

  Scenario: Enforces allowlist-only import policy and rejects blocked imports
    Given continue action code imports blocked packages
    When REPL import policy validation executes
    Then action fails with typed import-blocked error

  Scenario: Persists per-action artifact and sets node.action.executed.action_ref
    Given an action execution completes or fails
    When action artifact persistence executes
    Then artifact is persisted and node.action.executed.action_ref is set to canonical artifact reference

  Scenario: Returns exact action output fields from canonical current-run action_ref via read_action_artifact
    Given a node-local REPL session is initialized
    And a canonical current-run action artifact with exact stdout and stderr is persisted
    When read_action_artifact is invoked in REPL with that action_ref
    Then exact action output fields are returned to REPL context

  Scenario: Fails run on fatal REPL infrastructure errors with typed error metadata
    Given fatal REPL infrastructure failure occurs
    When harness handles failure propagation
    Then run transitions to failed with typed error metadata

  Scenario: Returns structured per-item batched results for llm_query_batched and rlm_query_batched
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Exposes read_action_artifact helper in node-local REPL session
    Given a node-local REPL session is initialized
    And a canonical current-run action artifact with exact stdout and stderr is persisted
    When read_action_artifact is invoked in REPL with that action_ref
    Then exact action output fields are returned to REPL context

  Scenario: Emits node.subcall.executed for each subcall item executed inside continue action
    Given canonical v1 run-event validation rules
    When canonical runtime event types are validated
    Then only canonical v1 runtime event types are accepted

  Scenario: Executes recursive subcalls with independent 300-second timeout budget decoupled from parent and recursive-level elapsed deadlines
    Given recursive subcall timeout budget is configured to 300 seconds
    And action timeout budget is configured to 180 seconds
    When recursive subcall timeout budgets are observed
    Then recursive subcall timeout budget is independent of parent action and recursive-level elapsed deadlines

  Scenario: Cancels recursive subcalls on run-context cancellation despite timeout decoupling from parent action context
    Given recursive subcall execution is in progress
    When run context is canceled during recursive subcall
    Then recursive subcall execution is canceled by run context

  Scenario: Keeps full raw context in REPL scope and excludes it from model-step inference messages
    Given a harness runner is configured with raw context "needle in haystack context"
    When model-step inference input is constructed for first step
    Then full raw context is excluded from outbound model-step inference messages

  Scenario: Builds model-step inference input as ordered role-based messages system then user
    Given a harness runner is configured with raw context "needle in haystack context"
    When model-step inference input is constructed for first step
    Then model-step inference messages are ordered system then user

  Scenario: Constructs deterministic user step envelope with query step index and context metadata
    Given a harness runner is configured with raw context "needle in haystack context"
    When model-step inference input is constructed for first step
    Then user step envelope contains deterministic query step index and context metadata

  Scenario: Includes execution_state with depth step budgets and recursion-permission metadata in user step envelope
    Given a harness runner is configured with raw context "needle in haystack context"
    When model-step inference input is constructed for first step
    Then user step envelope includes execution_state with depth step budgets and recursion-permission metadata

  Scenario: Includes bounded previous-action feedback summary with action_ref and preview truncation metadata
    Given a harness runner has previous continue action feedback
    When model-step inference input is constructed for next step
    Then previous-action feedback summary includes action_ref and bounded preview truncation metadata

  Scenario: Includes previous_action_feedback.subcall_summary with deterministic counts by execution mode and status
    Given a harness runner has previous continue action subcall feedback
    When model-step inference input is constructed for next step
    Then previous-action feedback includes deterministic subcall summary counts

  Scenario: Omits previous-action feedback block on first step before any continue action executes
    Given a harness runner is configured with raw context "needle in haystack context"
    When model-step inference input is constructed for first step
    Then previous-action feedback block is omitted from user step envelope

  Scenario: Persists compact node.turn.user artifact without embedding full raw context
    Given harness user turn artifact input is prepared
    When compact node turn user artifact is persisted
    Then persisted node turn user artifact excludes full raw context body

  Scenario: Preserves action artifact as source of truth for full stdout and stderr while model receives bounded previews
    Given a harness runner has previous continue action feedback
    When model-step inference input is constructed for next step
    Then action artifact remains source of truth for full stdout and stderr while model input uses bounded previews

  Scenario: Preserves bounded previous_action_feedback previews while exact action output remains recoverable through read_action_artifact
    Given a harness runner has previous continue action feedback
    When model-step inference input is constructed for next step
    Then previous-action feedback summary includes action_ref and bounded preview truncation metadata
    And exact action stdout remains recoverable through read_action_artifact using that action_ref

  Scenario: Constructs OpenRouter Responses API requests with message-array input preserving role order
    Given a valid inference request for execution
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference request construction runs
    Then request uses message-array input preserving role order

  Scenario: Applies bounded model-input contract consistently to recursive child nodes
    Given recursive harness child node execution input is prepared
    When model-step inference input is constructed for child step
    Then bounded model-input contract is applied to recursive child node step

  Scenario: Preserves non-recursive profile behavior under bounded model-input contract
    Given non-recursive harness mode is active
    When model-step inference input is constructed for non-recursive step
    Then bounded model-input contract is applied in non-recursive mode

  Scenario: Fails run with typed infrastructure metadata when step-envelope serialization or persistence fails
    Given step-envelope serialization or persistence failure is injected
    When harness run execution handles bounded model-input failure
    Then run fails with typed infrastructure metadata for bounded model-input failure

  Scenario: Maintains canonical run event ordering and references under bounded model-input execution
    Given bounded model-input execution is active for a node step
    When step and turn events are persisted under bounded model-input execution
    Then canonical run event ordering and references remain valid

  Scenario: Exposes llm_query llm_query_batched and rlm_query_batched in node-local Go REPL
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Executes llm_query as plain one-shot inference without child node creation
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Executes llm_query_batched with bounded parallel fan-out and preserves input-order results
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Executes rlm_query_batched sequentially and preserves input-order results
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Returns structured per-item batched results with answer and typed error metadata
    Given a node-local REPL session is initialized
    When REPL bindings are inspected
    Then rlm_query(prompt, context) is available and returns answer plus error

  Scenario: Routes subcalls through active node llm provider and model configuration
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration is merged
    Then effective run llm.provider is "openai"
    And effective run llm.model is "gpt-5.1"

  Scenario: Falls back to llm_query_batched item execution when rlm_query_batched reaches max depth in recursive mode
    Given an active parent node at depth 3
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then plain subcall fallback answer is returned and child node is not created

  Scenario: Preserves typed depth-limit behavior for recursive subcalls in non-recursive profile
    Given non-recursive harness mode is active
    When rlm_query is invoked from node-local Go REPL context
    Then typed depth-limit error is returned and child node is not created

  Scenario: Emits node.subcall.executed event for each subcall item with strict payload invariants
    Given canonical v1 run-event validation rules
    When canonical runtime event types are validated
    Then only canonical v1 runtime event types are accepted

  Scenario: Persists subcall traces in action artifacts with stable subcall indexing
    Given an action execution completes or fails
    When action artifact persistence executes
    Then artifact is persisted and node.action.executed.action_ref is set to canonical artifact reference

  Scenario: Allows multiple subcalls inside one continuation action while preserving one action per continue step
    Given a node-local step with decision continue
    When continuation payload is validated
    Then exactly one executable action is accepted for that step

  Scenario: Resolves plain-subcall responses through strict schema sigil.llm.answer.v1
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-valid"
    When inference request construction runs
    Then schema is resolved from central registry and applied to request

  Scenario: Fails run on subcall event persistence failures with typed infrastructure metadata
    Given fatal REPL infrastructure failure occurs
    When harness handles failure propagation
    Then run transitions to failed with typed error metadata

  Scenario: Surfaces plain-subcall inference failures as structured subcall result errors without immediate run failure
    Given a continue action fails with non-fatal REPL execution error
    When action failure is handled
    Then action failure is recorded and node execution continues to next step

  Scenario: Applies independent 300-second recursive timeout budget across recursive subcall levels for rlm_query and rlm_query_batched invocations
    Given recursive subcall timeout budget is configured to 300 seconds
    And action timeout budget is configured to 180 seconds
    When recursive subcall timeout budgets are observed
    Then recursive subcall timeout budget is independent of parent action and recursive-level elapsed deadlines

  Scenario: Requires continuation intent and expected_observation fields when decision is continue
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "continue-missing-intent"
    When inference execution runs
    Then inference fails with typed error code "output_validation"

  Scenario: Requires final evidence array when decision is final
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "final-missing-evidence"
    When inference execution runs
    Then inference fails with typed error code "output_validation"

  Scenario: Restricts final confidence to enum low medium or high when present
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "final-invalid-confidence"
    When inference execution runs
    Then inference fails with typed error code "output_validation"

  Scenario: Persists context artifact reference in context_metadata for model-step envelopes
    Given recursive harness child node execution input is prepared
    When model-step inference input is constructed for child step
    Then bounded model-input contract is applied to recursive child node step

  Scenario: Requires continuation intent expected_observation and repl_code in continue branch
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "continue-missing-intent"
    When inference execution runs
    Then inference fails with typed error code "output_validation"

  Scenario: Requires final answer evidence and optional confidence enum in final branch
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "valid-final"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Rejects unknown fields and malformed evidence entries under strict schema
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "unknown-field"
    When inference execution runs
    Then unknown fields are rejected with typed output-validation error

  Scenario: Persists node context artifact and includes context_ref in step input context_metadata
    Given recursive harness child node execution input is prepared
    When model-step inference input is constructed for child step
    Then bounded model-input contract is applied to recursive child node step

  Scenario: Exposes canonical and resolvable evidence references through context_ref and previous_action_feedback.action_ref
    Given a harness runner has previous continue action feedback
    When model-step inference input is constructed for next step
    Then previous-action feedback summary includes action_ref and bounded preview truncation metadata

  Scenario: Validates final evidence references against run-local persisted artifacts before node completion
    Given an active root node inference result is decision final with answer "root final"
    When harness evaluates root node step
    Then run completion references terminal root final output

  Scenario: Fails run with typed output-validation metadata when any final evidence reference cannot be resolved
    Given step-envelope serialization or persistence failure is injected
    When harness run execution handles bounded model-input failure
    Then run fails with typed infrastructure metadata for bounded model-input failure

  Scenario: Accepts final evidence references for the canonical artifact scheme
    Given an action execution completes or fails
    When action artifact persistence executes
    Then artifact is persisted and node.action.executed.action_ref is set to canonical artifact reference

  Scenario: Persists enriched final-answer artifact with answer evidence and optional confidence
    Given an active root node inference result is decision final with answer "root final"
    When harness evaluates root node step
    Then run completion references terminal root final output

  Scenario: Generates system prompt schema block from central registry definition sigil.rlm.response.v1 at runtime
    Given harness base system prompt resolution runs
    When harness effective system prompt is constructed
    Then effective system prompt equals resolved base prompt

  Scenario: Applies provider prompt resolution and system_prompt_append composition after runtime schema rendering
    Given harness base system prompt resolution runs
    And run config system_prompt_append is "Use concise tool calls."
    When harness effective system prompt is constructed
    Then effective system prompt equals base prompt plus two newlines plus append text

  Scenario: Preserves one-action-per-continue-step and recursion profile semantics under enriched schema
    Given a node-local step with decision continue
    When continuation payload is validated
    Then exactly one executable action is accepted for that step

  Scenario: Maintains inference schema_id sigil.rlm.response.v1 after schema extension
    Given a valid inference request for execution
    And inference request schema_id is "sigil.rlm.response.v1"
    When inference request construction runs
    Then schema is resolved from central registry and applied to request

  Scenario: Maintains prompt-schema parity by deriving prompt schema text from the same registry definition used for strict inference validation
    Given harness base system prompt resolution runs
    When harness effective system prompt is constructed
    Then effective system prompt equals resolved base prompt

  Scenario: Requires byte-for-byte previous_action_feedback.action_ref reuse with context_ref fallback for final evidence citations
    Given harness base system prompt resolution runs
    When harness effective system prompt is constructed
    Then system prompt requires byte-for-byte previous_action_feedback.action_ref reuse with context_ref fallback

  Scenario: Defines node.failed event type for canonical failed-node terminalization
    Given canonical v1 run-event validation rules
    When canonical runtime event types are validated
    Then only canonical v1 runtime event types are accepted

  Scenario: Enforces strict payload schema and invariants for node.failed events
    Given canonical v1 run-event validation rules
    When canonical runtime event types are validated
    Then only canonical v1 runtime event types are accepted

  Scenario: Enforces node terminal-event exclusivity between node.completed and node.failed
    Given an active root node inference result is decision final with answer "final answer"
    When harness evaluates root node step
    Then run completion references terminal root final output

  Scenario: Applies schema-specific raw-text fallback for sigil.llm.answer.v1 extraction failures
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-raw-text"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Rejects raw-text fallback for non sigil.llm.answer.v1 schemas
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "invalid-json-text"
    When inference execution runs
    Then inference fails with typed error code "gateway_failure"

  Scenario: Emits extraction-mode metadata in raw_metadata when plain-subcall fallback is applied
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-raw-text"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Persists structured compile diagnostics in failed action artifacts for repl_execution_compile errors
    Given a continue action fails with non-fatal REPL execution error
    When action failure is handled
    Then action failure is recorded and node execution continues to next step

  Scenario: Propagates compile diagnostics into previous-action feedback for subsequent model steps
    Given a continue action fails with non-fatal REPL execution error
    When action failure is handled
    Then action failure is recorded and node execution continues to next step

  Scenario: Includes optional previous_action_feedback.error_detail for compile-stage failures
    Given a harness runner has previous continue action feedback
    When model-step inference input is constructed for next step
    Then previous-action feedback summary includes action_ref and bounded preview truncation metadata

  Scenario: Applies raw-text fallback only for plain-subcall schema sigil.llm.answer.v1
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-raw-text"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Rejects raw-text fallback for non plain-subcall schemas in subcall execution paths
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.rlm.response.v1"
    And openrouter mock gateway returns payload fixture "invalid-json-text"
    When inference execution runs
    Then inference fails with typed error code "gateway_failure"

  Scenario: Defines node.failed as canonical node terminal event for failed node executions
    Given fatal REPL infrastructure failure occurs
    When harness handles failure propagation
    Then run transitions to failed with typed error metadata

  Scenario: Emits exactly one terminal node event node.completed or node.failed for every node.started
    Given an active root node inference result is decision final with answer "final answer"
    When harness evaluates root node step
    Then run completion references terminal root final output

  Scenario: Emits node.failed for recursive child execution failures before parent node.subcall.executed failed record
    Given an active parent node at depth 1
    And run max recursion depth is 3
    When rlm_query is invoked from node-local Go REPL context
    Then child node is created at depth 2

  Scenario: Emits node.failed for root execution failures before run.failed
    Given fatal REPL infrastructure failure occurs
    When harness handles failure propagation
    Then run transitions to failed with typed error metadata

  Scenario: Preserves run continuation when child node.failed is surfaced as repl_child_failure in parent continue action path
    Given a continue action fails with non-fatal REPL execution error
    When action failure is handled
    Then action failure is recorded and node execution continues to next step

  Scenario: Applies schema-specific raw-text fallback for sigil.llm.answer.v1 when strict structured payload extraction fails
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-raw-text"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Emits extraction-mode metadata in normalized inference raw_metadata for plain-subcall responses
    Given a valid inference request for execution
    And central inference schema registry is initialized
    And inference request schema_id is "sigil.llm.answer.v1"
    And openrouter mock gateway returns payload fixture "llm-answer-raw-text"
    When inference execution runs
    Then normalized output contains all required canonical fields

  Scenario: Includes compile diagnostics in previous_action_feedback for subsequent model steps
    Given a harness runner has previous continue action feedback
    When model-step inference input is constructed for next step
    Then previous-action feedback summary includes action_ref and bounded preview truncation metadata

  Scenario: Preserves node.action.executed payload contract while exposing diagnostics through artifact and feedback only
    Given an action execution completes or fails
    When action artifact persistence executes
    Then artifact is persisted and node.action.executed.action_ref is set to canonical artifact reference

  Scenario: Preserves one-action-per-continue-step and subcall observability contracts under hardening changes
    Given a node-local step with decision continue
    When continuation payload is validated
    Then exactly one executable action is accepted for that step

  Scenario: Defines deterministic guardrails section in run config schema including optional accounting budgets
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      guardrails:
        max_steps_per_node: 10
        max_total_steps_per_run: 20
        max_run_duration_ms: 30000
        max_consecutive_step_failures: 2
        max_total_tokens: 99
        max_total_cost_usd: "0.25"
      """
    When run configuration validation runs
    Then run configuration initialization succeeds

  Scenario: Applies deterministic guardrail defaults when guardrails fields are omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run guardrails.max_steps_per_node is 64
    And effective run guardrails.max_total_steps_per_run is 256
    And effective run guardrails.max_run_duration_ms is 1200000
    And effective run guardrails.max_consecutive_step_failures is 6
    And effective run guardrails.max_total_tokens is unset
    And effective run guardrails.max_total_cost_usd is unset

  Scenario: Applies SIGIL_RUN environment overrides for deterministic guardrail fields
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      guardrails:
        max_steps_per_node: 10
        max_total_steps_per_run: 20
        max_run_duration_ms: 30000
        max_consecutive_step_failures: 2
        max_total_tokens: 99
        max_total_cost_usd: "0.25"
      """
    And environment override "SIGIL_RUN_GUARDRAILS_MAX_STEPS_PER_NODE" is "64"
    And environment override "SIGIL_RUN_GUARDRAILS_MAX_TOTAL_STEPS_PER_RUN" is "256"
    And environment override "SIGIL_RUN_GUARDRAILS_MAX_RUN_DURATION_MS" is "1200000"
    And environment override "SIGIL_RUN_GUARDRAILS_MAX_CONSECUTIVE_STEP_FAILURES" is "6"
    And environment override "SIGIL_RUN_GUARDRAILS_MAX_TOTAL_TOKENS" is "512"
    And environment override "SIGIL_RUN_GUARDRAILS_MAX_TOTAL_COST_USD" is "001.230000"
    When run configuration is merged
    Then effective run guardrails.max_steps_per_node is 64
    And effective run guardrails.max_total_steps_per_run is 256
    And effective run guardrails.max_run_duration_ms is 1200000
    And effective run guardrails.max_consecutive_step_failures is 6
    And effective run guardrails.max_total_tokens is 512
    And effective run guardrails.max_total_cost_usd is "1.23"

  Scenario: Rejects run configuration when deterministic guardrail values are invalid
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      guardrails:
        max_steps_per_node: 0
        max_total_steps_per_run: 5
        max_run_duration_ms: 30000
        max_consecutive_step_failures: 2
        max_total_tokens: 0
        max_total_cost_usd: "1.2345678"
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Extends run.failed payload with deterministic guardrail metadata fields
    Given run.failed payload includes deterministic guardrail metadata
    When strict payload schema validation is executed for run.failed
    Then run.failed payload validation succeeds

  Scenario: Requires configured_value and observed_value when run.failed limit_key is present
    Given run.failed payload includes limit_key without configured_value or observed_value
    When strict payload schema validation is executed for run.failed
    Then run.failed payload validation fails

  Scenario: Validates failed_step_id as UUIDv7 when present in run.failed payload
    Given run.failed payload includes non-uuidv7 failed_step_id
    When strict payload schema validation is executed for run.failed
    Then run.failed payload validation fails

  Scenario: Applies deterministic runtime guardrails from PRD-0500 during harness execution
    Given deterministic runtime guardrail fixture "max_steps_per_node" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_steps_per_node"

  Scenario: Exits non-zero when deterministic runtime guardrail breach terminalizes run
    Given deterministic runtime guardrail fixture "max_steps_per_node" is prepared
    When a user runs guardrail-breach harness entrypoint
    Then command exits non-zero

  Scenario: Defines run guardrails config section for deterministic step time failure and accounting budget thresholds
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      guardrails:
        max_steps_per_node: 1
        max_total_steps_per_run: 2
        max_run_duration_ms: 1000
        max_consecutive_step_failures: 1
        max_total_tokens: 99
        max_total_cost_usd: "0.25"
      """
    When run configuration validation runs
    Then run configuration initialization succeeds

  Scenario: Applies default deterministic guardrail values when guardrails config is omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run guardrails.max_steps_per_node is 64
    And effective run guardrails.max_total_steps_per_run is 256
    And effective run guardrails.max_run_duration_ms is 1200000
    And effective run guardrails.max_consecutive_step_failures is 6
    And effective run guardrails.max_total_tokens is unset
    And effective run guardrails.max_total_cost_usd is unset

  Scenario: Rejects invalid deterministic guardrail configuration values
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      guardrails:
        max_steps_per_node: 0
        max_total_steps_per_run: 1
        max_run_duration_ms: 1000
        max_consecutive_step_failures: 1
        max_total_tokens: 0
        max_total_cost_usd: "1.2345678"
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Enforces max_steps_per_node before appending node.step.started
    Given deterministic runtime guardrail fixture "max_steps_per_node" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_steps_per_node"

  Scenario: Enforces max_total_steps_per_run across root and recursive nodes
    Given deterministic runtime guardrail fixture "max_total_steps_per_run" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_total_steps_per_run"

  Scenario: Enforces max_run_duration_ms as hard run wall-clock budget
    Given deterministic runtime guardrail fixture "max_run_duration_ms" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_run_duration_ms"
    And max_run_duration_ms interrupts the active step before completion

  Scenario: Enforces max_consecutive_step_failures using consecutive failed continue actions
    Given deterministic runtime guardrail fixture "max_consecutive_step_failures" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_consecutive_step_failures"

  Scenario: Resets consecutive failed-step counter after successful continue action or final decision
    Given deterministic runtime guardrail fixture "consecutive_failure_reset" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail reset fixture completes successfully

  Scenario: Emits run.failed with harness_limit_exceeded and deterministic limit metadata on breach
    Given deterministic runtime guardrail fixture "max_steps_per_node" is prepared
    When deterministic runtime guardrail harness run executes
    Then run.failed includes deterministic runtime guardrail metadata

  Scenario: Includes failed_node_id and optional failed_step_id in run.failed guardrail failures
    Given deterministic runtime guardrail fixture "max_steps_per_node" is prepared
    When deterministic runtime guardrail harness run executes
    Then run.failed includes failed_node_id and optional failed_step_id for guardrail breaches

  Scenario: Applies deterministic guardrails identically in recursive and non-recursive profiles
    Given deterministic runtime guardrail fixture "recursive_profile_parity" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail parity is preserved for recursive and non-recursive profiles

  Scenario: Enforces max_total_tokens on cumulative run accounting tree totals
    Given deterministic runtime guardrail fixture "max_total_tokens" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_total_tokens"

  Scenario: Enforces max_total_cost_usd on cumulative run accounting tree totals
    Given deterministic runtime guardrail fixture "max_total_cost_usd" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail breach uses limit key "max_total_cost_usd"

  Scenario: Fails closed when max_total_tokens sees incomplete tree-total accounting
    Given deterministic runtime guardrail fixture "max_total_tokens_incomplete" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail failure requires complete accounting for limit key "max_total_tokens" with status "partial" and observed value "5"

  Scenario: Fails closed when max_total_cost_usd sees incomplete tree-total accounting
    Given deterministic runtime guardrail fixture "max_total_cost_usd_incomplete" is prepared
    When deterministic runtime guardrail harness run executes
    Then deterministic runtime guardrail failure requires complete accounting for limit key "max_total_cost_usd" with status "unavailable" and observed value "unavailable"

  Scenario: Defines accounting section in run config schema
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      accounting:
        pricing_version: custom-v1
        fallback_pricing:
          openai:
            gpt-5.1:
              input_microusd_per_million_tokens: 111
              output_microusd_per_million_tokens: 222
              reasoning_microusd_per_million_tokens: 333
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run accounting.pricing_version is "custom-v1"
    And effective run accounting fallback pricing for provider "openai" model "gpt-5.1" uses input rate 111 output rate 222 reasoning rate 333

  Scenario: Applies default accounting pricing version when accounting section is omitted
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      """
    When run configuration validation runs
    Then run configuration initialization succeeds
    And effective run accounting.pricing_version is "v1"

  Scenario: Applies SIGIL_RUN environment overrides for accounting fallback pricing
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      accounting:
        pricing_version: file-v1
      """
    And environment override "SIGIL_RUN_ACCOUNTING_PRICING_VERSION" is "env-v2"
    And environment override "SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_INPUT_MICROUSD_PER_MILLION_TOKENS" is "111"
    And environment override "SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_OUTPUT_MICROUSD_PER_MILLION_TOKENS" is "222"
    And environment override "SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_REASONING_MICROUSD_PER_MILLION_TOKENS" is "333"
    When run configuration is merged
    Then effective run accounting.pricing_version is "env-v2"
    And effective run accounting fallback pricing for provider "openai" model "gpt-5.1" uses input rate 111 output rate 222 reasoning rate 333

  Scenario: Rejects run configuration when accounting fallback pricing values are non-positive
    Given run configuration exists at "./sigil-run.yaml" with:
      """
      name: test-run
      prompt: prompt
      context: context
      llm:
        provider: openai
        model: gpt-5.1
      accounting:
        fallback_pricing:
          openai:
            gpt-5.1:
              input_microusd_per_million_tokens: 0
              output_microusd_per_million_tokens: 1
      """
    When run configuration validation runs
    Then run configuration initialization fails

  Scenario: Includes accounting rollup in successful run summary and terminal events
    Given a harness runner captures provider-reported accounting for a final step
    When accounting artifacts are persisted for the completed run
    Then successful run summary and terminal events include accounting

  Scenario: Computes fallback cost from configured pricing when gateway omits cost
    Given a harness runner captures fallback-priced accounting for a final step
    When accounting rollups are inspected
    Then fallback pricing-derived accounting cost is preserved in completed run accounting

  Scenario: Aggregates recursive subcall tree accounting into node and run totals
    Given a recursive harness run captures subtree accounting
    When accounting rollups are inspected
    Then recursive accounting tree total includes child node totals

  Scenario: Persists subcall leaf accounting in events and action artifacts
    Given a recursive harness run captures subtree accounting
    When accounting rollups are inspected
    Then subcall events and action artifacts include leaf accounting summaries

  Scenario: Preserves partial accounting status when cost is unavailable
    Given a harness runner captures partial accounting for a final step
    When accounting rollups are inspected
    Then partial accounting totals remain marked partial instead of zero-complete

  Scenario: Includes partial accounting in failed terminal events
    Given a running lifecycle captures partial terminal accounting
    When a failed terminal event is persisted with accounting
    Then failed terminal events include partial accounting

  Scenario: Includes partial accounting in interrupted terminal events
    Given a running lifecycle captures partial terminal accounting
    When an interrupted terminal event is persisted with accounting
    Then interrupted terminal events include partial accounting
