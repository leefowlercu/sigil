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
    And baseline application config keys are "log_level" and "log_dir"

  Scenario: Applies defaults and SIGIL environment overrides for log_level and log_dir
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: debug
      """
    And environment override "SIGIL_LOG_LEVEL" is "warn"
    And environment override "SIGIL_LOG_DIR" is "./env-logs"
    When application configuration is merged
    Then effective application log_level is "warn"
    And effective application log_dir is "./env-logs"

  Scenario: Rejects unsupported log_level values
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: trace
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

  Scenario: Defers sigil run start behavior to PRD-0004 run-start config-input contract
    Given the sigil executable is available
    And no default start config files exist
    When a user runs `sigil run start`
    Then command exits non-zero
    And command error contains `invalid --config value`

  Scenario: Provides sigil run stop as a usage-only placeholder command
    Given the sigil executable is available
    When a user runs `sigil run stop`
    Then stop usage/help is printed
    And command exits with status code 0

  Scenario: Uses framework-default error behavior for unknown subcommands
    Given the sigil executable is available
    When a user runs `sigil unknown`
    Then command exits non-zero
    And command error contains `unknown command`

  Scenario: Loads default run configuration file from current working directory
    Given run configuration exists at "./sigil-run.yaml" with:
      """
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
      log_level: info
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start`
    Then command exits with status code 0

  Scenario: Overrides application configuration path with --config
    Given application config exists at "./custom-sigil.yaml" with:
      """
      log_level: warn
      """
    And run config file exists at "./sigil-run.yaml"
    When a user runs `sigil run start --config ./custom-sigil.yaml`
    Then command exits with status code 0

  Scenario: Overrides run configuration path with --run-config
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: info
      """
    And run config file exists at "./custom-run.yaml"
    When a user runs `sigil run start --run-config ./custom-run.yaml`
    Then command exits with status code 0

  Scenario: Overrides both configuration paths when both flags are provided
    Given application config exists at "./custom-sigil.yaml" with:
      """
      log_level: debug
      """
    And run config file exists at "./custom-run.yaml"
    When a user runs `sigil run start --config ./custom-sigil.yaml --run-config ./custom-run.yaml`
    Then command exits with status code 0

  Scenario: Fails when required configuration paths are missing unreadable or not regular files
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: info
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

  Scenario: Writes application logs to a derived sigil.log file path
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: info
      log_dir: ./derived-log-dir
      """
    When application logging is initialized
    Then the effective log file path is "./derived-log-dir/sigil.log"

  Scenario: Uses JSON structured log records for application logging
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: info
      log_dir: ./json-log-dir
      """
    When application logging is initialized
    And application logging writes an info record with message "json-acceptance-record"
    Then log records are structured JSON

  Scenario: Uses default log file path when default log_dir is in effect
    Given the sigil application starts without an explicit application config path
    When application logging is initialized
    Then the effective log target path is "./sigil/logs/sigil.log"

  Scenario: Fails initialization when derived log file path cannot be opened as a file sink
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: info
      log_dir: ./blocked-log-target
      """
    And a file exists at "./blocked-log-target"
    When a user runs `sigil run`
    Then command exits non-zero
    And command error contains `failed to initialize application logging`

  Scenario: Derives log file path from configured log_dir override
    Given application config exists at "./sigil.yaml" with:
      """
      log_level: info
      log_dir: ./override-log-dir
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

  Scenario: Persists run events to per-run append-only events.jsonl under sigil runs directory
    Given a persisted lifecycle run exists
    When canonical run lifecycle events are emitted
    Then events are persisted to a per-run append-only events.jsonl path under sigil runs directory

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

  Scenario: Defines canonical core lifecycle event type catalog for v1
    Given canonical v1 run-event validation rules
    When canonical core lifecycle event types are validated
    Then only canonical v1 lifecycle event types are accepted

  Scenario: Enforces strict payload schema and invariants for each core lifecycle event type
    Given canonical v1 lifecycle events with payloads
    When strict payload schema validation is executed
    Then required fields types and invariants are enforced per event type

  Scenario: Rejects unknown fields and unknown event types under v1 strict extensibility rules
    Given v1 event envelopes with unknown fields or unknown type
    When strict v1 extensibility validation is executed
    Then validation fails and events are rejected

  Scenario: Defers non-core tool and model payload families while keeping core lifecycle payloads normative
    Given a core lifecycle event payload includes deferred non-core fields
    When core v1 payload validation executes
    Then deferred non-core fields are rejected as out-of-contract

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

  Scenario: Requires continuation branch and forbids final branch when decision is continue
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
