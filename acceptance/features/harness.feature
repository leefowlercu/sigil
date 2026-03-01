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
