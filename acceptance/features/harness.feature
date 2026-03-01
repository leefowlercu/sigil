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

  Scenario: Fails when a resolved configuration path is missing or not a readable file
    Given run config file exists at "./sigil-run.yaml"
    And a directory exists at "./not-a-file"
    When a user runs `sigil run start --config ./not-a-file`
    Then command exits non-zero
    And command error contains `invalid --config value`
    When a user runs `sigil run start --unknown-flag`
    Then command exits non-zero
    And command error contains `unknown flag`
