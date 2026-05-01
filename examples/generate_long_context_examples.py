#!/usr/bin/env python3
"""Generate synthetic long-context examples for Sigil.

The generated examples are benchmark-inspired, but they do not copy benchmark
data. Each fixture is intentionally deterministic so the checked-in contexts,
questions, and expected answers can be regenerated during review.
"""

from __future__ import annotations

import json
from pathlib import Path
from textwrap import dedent


EXAMPLES_DIR = Path(__file__).resolve().parent


SIGIL_YAML = """logs:
  level: info
  dir: ./.sigil/logs
"""


def run_config(name: str, max_depth: int = 4, max_tokens: int = 1_500_000) -> str:
    return f"""name: {name}

prompt_template: |
  {{{{.question}}}}

context_template: |
  {{{{.external_context}}}}

llm:
  provider: openai
  model: gpt-5.3-codex
  gateway: openrouter
  reasoning:
    enabled: false
    effort: medium
  openrouter:
    base_url: https://openrouter.ai/api/v1
    request_timeout_ms: 180000
    api_key_env: OPENROUTER_API_KEY

rlm:
  enabled: true
  max_depth: {max_depth}

guardrails:
  max_steps_per_node: 128
  max_total_steps_per_run: 512
  max_run_duration_ms: 1800000
  max_consecutive_step_failures: 10
  max_total_tokens: {max_tokens}
  max_total_cost_usd: "10"

accounting:
  pricing_version: openrouter-2026-03-07
  fallback_pricing:
    openai:
      gpt-5.3-codex:
        input_microusd_per_million_tokens: 1750000
        output_microusd_per_million_tokens: 14000000
"""


def readme(example_name: str, title: str, summary: str, source: str) -> str:
    return dedent(
        f"""\
        # {example_name}

        {summary}

        This is a synthetic, checked-in fixture inspired by {source}. It is not
        copied from the upstream benchmark. The goal is to give Sigil a
        repeatable long-context example that stresses the same kind of harness
        behavior while keeping the answer contract small enough to inspect.

        The bundled `sigil-run.yaml` uses `openai/gpt-5.3-codex` via OpenRouter,
        with reasoning disabled, recursion enabled, and showcase-biased
        step/accounting guardrails.

        ## Prerequisites

        - `OPENROUTER_API_KEY` must be set in the shell that runs the example.
        - Build `sigil` first if `./sigil` is not already present.

        ## Default Human-Readable Run

        ```bash
        cd /Users/lee/Dev/project/project-sigil/sigil

        question_value="$(cat ./examples/{example_name}/question.txt)"
        context_value="$(cat ./examples/{example_name}/context.txt)"

        ./sigil run start \\
          --config ./examples/{example_name}/sigil.yaml \\
          --run-config ./examples/{example_name}/sigil-run.yaml \\
          --var question="$question_value" \\
          --var external_context="$context_value"
        ```

        ## Machine-Readable JSON Run

        ```bash
        cd /Users/lee/Dev/project/project-sigil/sigil

        question_value="$(cat ./examples/{example_name}/question.txt)"
        context_value="$(cat ./examples/{example_name}/context.txt)"

        ./sigil run start -o json \\
          --config ./examples/{example_name}/sigil.yaml \\
          --run-config ./examples/{example_name}/sigil-run.yaml \\
          --var question="$question_value" \\
          --var external_context="$context_value"
        ```

        ## Expected Answer Verification

        ```bash
        cd /Users/lee/Dev/project/project-sigil/sigil

        question_value="$(cat ./examples/{example_name}/question.txt)"
        context_value="$(cat ./examples/{example_name}/context.txt)"
        output_json="$(mktemp)"

        ./sigil run start -o json \\
          --config ./examples/{example_name}/sigil.yaml \\
          --run-config ./examples/{example_name}/sigil-run.yaml \\
          --var question="$question_value" \\
          --var external_context="$context_value" >"$output_json"

        python3 - "$output_json" ./examples/{example_name}/expected-answer.txt <<'PY'
        import json
        import pathlib
        import sys

        actual = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))["final_answer"].strip()
        expected = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8").strip()
        if actual != expected:
            raise SystemExit("final_answer mismatch")
        print("final_answer matches expected-answer.txt")
        PY
        ```

        See `benchmark-metadata.json` for fixture provenance and design notes.
        """
    )


def metadata(task_family: str, inspired_by: list[str], design_note: str) -> str:
    return json.dumps(
        {
            "source": "synthetic",
            "task_family": task_family,
            "inspired_by": inspired_by,
            "design_note": design_note,
            "scoring_note": "Compare final_answer to expected-answer.txt after trimming boundary whitespace.",
        },
        indent=2,
    ) + "\n"


def write_example(
    example_name: str,
    title: str,
    summary: str,
    source: str,
    task_family: str,
    inspired_by: list[str],
    design_note: str,
    question: str,
    context: str,
    expected_answer: str,
) -> None:
    path = EXAMPLES_DIR / example_name
    path.mkdir(parents=True, exist_ok=True)
    files = {
        "README.md": readme(example_name, title, summary, source),
        "benchmark-metadata.json": metadata(task_family, inspired_by, design_note),
        "context.txt": context.rstrip() + "\n",
        "expected-answer.txt": expected_answer.rstrip() + "\n",
        "question.txt": question.rstrip() + "\n",
        "sigil-run.yaml": run_config(example_name),
        "sigil.yaml": SIGIL_YAML,
    }
    for filename, content in files.items():
        (path / filename).write_text(content, encoding="utf-8")


def variable_tracking() -> tuple[str, str, str]:
    updates = {
        64: ("harbor_token", "HARBOR-ALPHA-113"),
        127: ("orbit_flag", "ORBIT-AMBER"),
        211: ("cedar_index", "CEDAR-204"),
        276: ("crimson_gate", "GATE-NORTH-72"),
        359: ("harbor_token", "HARBOR-SILVER-584"),
        433: ("orbit_flag", "ORBIT-VERDANT"),
        501: ("cedar_index", "CEDAR-731"),
        557: ("crimson_gate", "GATE-TIDAL-09"),
        612: ("harbor_token", "HARBOR-ONYX-447"),
        628: ("orbit_flag", "ORBIT-GLASS"),
    }
    lines = [
        "# Synthetic Variable Tracking Ledger",
        "Apply only explicit SET operations in ascending LOG order.",
        "Ignore checkpoint, prediction, rollback, rehearsal, and rumor lines.",
        "",
    ]
    distractor_values = [
        "HARBOR-CRANE-002",
        "ORBIT-MIST",
        "CEDAR-019",
        "GATE-WEST-41",
    ]
    for index in range(1, 641):
        log_id = f"LOG-{index:04d}"
        if index in updates:
            key, value = updates[index]
            lines.append(
                f"{log_id} | change-control | SET {key} = {value} | "
                "auditor=primary | status=accepted"
            )
            continue
        decoy = distractor_values[index % len(distractor_values)]
        lines.append(
            f"{log_id} | observation | checkpoint text mentions candidate {decoy}; "
            "no SET operation is present and no tracked variable changes here."
        )
        if index % 9 == 0:
            lines.append(
                f"{log_id} | rehearsal | operator rehearsed a future change but did not commit it; "
                "treat this as narrative noise."
            )
    question = (
        "After applying only explicit SET operations in chronological order, return exactly: "
        "harbor_token=<latest>; orbit_flag=<latest>; cedar_index=<latest>; "
        "crimson_gate=<latest>; evidence=<comma-separated LOG ids for those latest SET entries "
        "in the same field order>."
    )
    expected = (
        "harbor_token=HARBOR-ONYX-447; orbit_flag=ORBIT-GLASS; "
        "cedar_index=CEDAR-731; crimson_gate=GATE-TIDAL-09; "
        "evidence=LOG-0612,LOG-0628,LOG-0501,LOG-0557"
    )
    return question, "\n".join(lines), expected


def frequency_aggregation() -> tuple[str, str, str]:
    target_counts = {
        "sig-river": 43,
        "sig-lantern": 38,
        "sig-copper": 31,
        "sig-violet": 26,
        "sig-harbor": 19,
        "sig-saffron": 17,
    }
    signal_tokens: list[str] = []
    for token, count in target_counts.items():
        signal_tokens.extend([token] * count)

    packets: list[list[str]] = [[] for _ in range(512)]
    for index, token in enumerate(signal_tokens):
        packets[((index * 37) + 11) % len(packets)].append(token)

    lines = [
        "# Synthetic Frequency Aggregation Packets",
        "Count only tokens on lines beginning with 'signals:' that start with 'sig-'.",
        "Ignore stoplist tokens and tokens beginning with 'decoy-'.",
        "",
    ]
    stoplist = ["noise", "blank", "hold", "ping"]
    decoys = ["decoy-river", "decoy-lantern", "decoy-copper", "decoy-violet"]
    for index, packet in enumerate(packets, start=1):
        packet_id = f"PACKET-{index:04d}"
        fillers = [stoplist[index % 4], stoplist[(index + 1) % 4], decoys[index % 4]]
        if index % 5 == 0:
            fillers.append(decoys[(index + 1) % 4])
        rendered = ", ".join(packet + fillers)
        lines.append(f"{packet_id}")
        lines.append(f"signals: {rendered}")
        lines.append(
            "note: summary text may name colors and waterways, but only the signals line is authoritative."
        )
        lines.append("")
    question = (
        "Count every token on `signals:` lines whose token begins with `sig-`. "
        "Ignore stoplist tokens and `decoy-` tokens. Return the top three by descending frequency "
        "exactly as: rank1=<token>:<count>; rank2=<token>:<count>; rank3=<token>:<count>."
    )
    expected = "rank1=sig-river:43; rank2=sig-lantern:38; rank3=sig-copper:31"
    return question, "\n".join(lines), expected


def longbench_multidoc() -> tuple[str, str, str]:
    special_docs = {
        47: (
            "Polar Relay hard requirements",
            "The selected field-terminal vendor must support sealed offline mode, "
            "avoid biometric access, operate at or below -25 C, and keep first-year "
            "cost at or below 180000.",
        ),
        121: (
            "Northstar Lattice technical note",
            "Northstar Lattice supports sealed offline mode for isolated outposts, "
            "runs through a -30 C cold-start test, and keeps an audited local export "
            "queue while disconnected.",
        ),
        198: (
            "Privacy addendum",
            "Northstar Lattice uses a non-biometric keypad. Helio Prism requires a "
            "fingerprint unlock, and Meridian Atlas uses face unlock during supervisor override.",
        ),
        233: (
            "Commercial comparison",
            "Northstar Lattice has a first-year cost of 172000. Helio Prism is 165000, "
            "Meridian Atlas is 151000, and Shoreline Kit is 181500.",
        ),
    }
    lines = ["# Synthetic Multi-Document QA Corpus", ""]
    vendors = ["Helio Prism", "Meridian Atlas", "Shoreline Kit", "Cobalt Fieldbox"]
    for index in range(1, 257):
        doc_id = f"DOC-{index:04d}"
        if index in special_docs:
            title, body = special_docs[index]
        else:
            vendor = vendors[index % len(vendors)]
            title = f"{vendor} regional note {index:02d}"
            body = (
                f"{vendor} has a useful property in one region, but this document does not "
                "establish all Polar Relay hard requirements. Treat marketing claims, pilot "
                "rumors, and partial discounts as insufficient unless another document provides "
                "the missing requirement."
            )
        lines.append(f"[{doc_id}] {title}")
        lines.append(body)
        lines.append("")
    question = (
        "Which vendor satisfies Polar Relay's hard requirements? Return exactly: "
        "vendor=<name>; constraints=<comma-separated facts>; evidence=<doc-ids in ascending order>."
    )
    expected = (
        "vendor=Northstar Lattice; constraints=sealed offline mode, non-biometric access, "
        "-30 C operation, 172000 cost; evidence=DOC-0047,DOC-0121,DOC-0198,DOC-0233"
    )
    return question, "\n".join(lines), expected


def nolima_semantic_needle() -> tuple[str, str, str]:
    lines = [
        "# Synthetic Semantic Needle Corpus",
        "The answer may be described with different words than the question uses.",
        "",
    ]
    names = ["Ari", "Jun", "Lena", "Oren", "Priya", "Sol", "Tamsin", "Vera"]
    objects = ["map tube", "tin lantern", "red satchel", "cedar crate", "folded tarp"]
    recipients = ["ferry pilot", "schoolmaster", "dock steward", "radio clerk", "market scribe"]
    for index in range(1, 361):
        card_id = f"CARD-{index:04d}"
        if index == 248:
            body = (
                "Mira handled the rescue cache after the siren drill. She wrapped the "
                "sugar-crash medicine in oilcloth, slid the small blue vial into the hollow "
                "brass compass, and entrusted the compass to the station clockkeeper before dawn."
            )
        else:
            name = names[index % len(names)]
            obj = objects[index % len(objects)]
            recipient = recipients[index % len(recipients)]
            body = (
                f"{name} moved a routine supply bundle through the depot, checked a {obj}, "
                f"and gave the harmless paperwork to the {recipient}. This card is background "
                "traffic, not the requested medical handoff."
            )
            if index % 11 == 0:
                body += " Mira is mentioned in a schedule margin, but no container or recipient is specified."
        lines.append(f"[{card_id}]")
        lines.append(body)
        lines.append("")
    question = (
        "Where was the emergency insulin ampoule hidden before the handoff, and who received it? "
        "The context may describe the medicine indirectly. Return exactly: "
        "container=<container>; recipient=<recipient>; evidence=<card-id>."
    )
    expected = "container=hollow brass compass; recipient=station clockkeeper; evidence=CARD-0248"
    return question, "\n".join(lines), expected


def helmet_citation_rag() -> tuple[str, str, str]:
    special_docs = {
        "POL-004": (
            "Offline cache release policy",
            "Offline cache may ship only when revocation-ledger synchronization is scheduled "
            "within 24 hours and the audit export includes per-record hashes.",
        ),
        "REL-009": (
            "Delta build 14.8 release note",
            "Delta build 14.8 syncs the revocation ledger every 18 hours and exports a SHA-256 "
            "hash for each cached record. This note supersedes the older beta limitation.",
        ),
        "RISK-017": (
            "Kestrel clinic rollout risk",
            "Kestrel clinics require printed override cards until staff training completes on June 3.",
        ),
    }
    lines = ["# Synthetic Citation RAG Corpus", ""]
    filler_ids = [f"REF-{index:03d}" for index in range(1, 261)]
    all_ids = filler_ids[:70] + ["POL-004"] + filler_ids[70:160] + ["REL-009"] + filler_ids[160:230] + ["RISK-017"] + filler_ids[230:]
    for index, doc_id in enumerate(all_ids, start=1):
        if doc_id in special_docs:
            title, body = special_docs[doc_id]
        else:
            title = f"Operational reference {index:02d}"
            body = (
                "This reference covers staffing, telemetry labels, or older rollout assumptions. "
                "It does not jointly establish the offline-cache release decision for Team Delta."
            )
            if index % 13 == 0:
                body += " A previous beta concern is noted here, but it is not the current build note."
        lines.append(f"[{doc_id}] {title}")
        lines.append(body)
        lines.append("")
    question = (
        "Can Team Delta ship offline cache to Kestrel clinics on May 12? Return exactly: "
        "decision=<go|no-go|conditional-go>; rationale=<short>; citations=<ids>."
    )
    expected = (
        "decision=conditional-go; rationale=Delta build meets the offline-cache sync and "
        "hash-export policy, but Kestrel clinics require printed override cards until training "
        "completes on June 3; citations=POL-004,REL-009,RISK-017"
    )
    return question, "\n".join(lines), expected


def bright_rag_reasoning() -> tuple[str, str, str]:
    special_notes = {
        "ISSUE-014": (
            "AX-19 incident analysis",
            "AX-19 occurs in quartz-indexer when arm64 workers run cache compaction while the "
            "mmap writer is enabled. x86 workers are not affected by this fault mode.",
        ),
        "MATRIX-006": (
            "Terra deployment matrix",
            "Terra production workers are arm64, cache compaction is enabled, and cache_schema "
            "is set to delta.",
        ),
        "PATCH-031": (
            "Quartz indexer patch",
            "Patch PATCH-031 ships quartz-indexer version 2.7.4. It routes arm64 compaction "
            "away from the mmap writer and is safe only when cache_schema=delta.",
        ),
    }
    lines = ["# Synthetic Multi-Hop Technical RAG Corpus", ""]
    filler_ids = [f"NOTE-{index:03d}" for index in range(1, 261)]
    all_ids = filler_ids[:54] + ["ISSUE-014"] + filler_ids[54:142] + ["MATRIX-006"] + filler_ids[142:220] + ["PATCH-031"] + filler_ids[220:]
    for index, note_id in enumerate(all_ids, start=1):
        if note_id in special_notes:
            title, body = special_notes[note_id]
        else:
            title = f"Compatibility note {index:02d}"
            body = (
                "This note discusses unrelated packages, older warning codes, or architectures "
                "that do not match Terra's current deployment. Do not use it as the final patch evidence."
            )
            if index % 10 == 0:
                body += " Patch PATCH-028 is mentioned for x86-only queues and is a distractor."
        lines.append(f"[{note_id}] {title}")
        lines.append(body)
        lines.append("")
    question = (
        "Which patch should Terra apply for AX-19, and what config precondition makes it safe? "
        "Return exactly: patch=<id>; package=<name>; version=<version>; "
        "precondition=<key=value>; evidence=<ids>."
    )
    expected = (
        "patch=PATCH-031; package=quartz-indexer; version=2.7.4; "
        "precondition=cache_schema=delta; evidence=ISSUE-014,MATRIX-006,PATCH-031"
    )
    return question, "\n".join(lines), expected


def main() -> None:
    examples = [
        (
            "ruler-variable-tracking",
            "RULER-style variable tracking",
            "This example asks the harness to track mutable state through a noisy ledger.",
            "RULER variable tracking",
            "state tracking",
            ["RULER variable tracking"],
            "The final answer depends on applying ordered state updates while ignoring decoy checkpoint text.",
            variable_tracking,
        ),
        (
            "ruler-frequency-aggregation",
            "RULER-style frequency aggregation",
            "This example asks the harness to count repeated signal tokens across many packets.",
            "RULER common/frequent words extraction",
            "aggregation",
            ["RULER common words extraction", "RULER frequent words extraction"],
            "The final answer depends on partition-friendly counting and top-k aggregation.",
            frequency_aggregation,
        ),
        (
            "longbench-multidoc",
            "LongBench-style multi-document QA",
            "This example asks the harness to combine constraints distributed across multiple documents.",
            "LongBench v2 multi-document QA",
            "multi-document question answering",
            ["LongBench v2 multi-document QA"],
            "The final answer depends on synthesizing compatible evidence across several documents.",
            longbench_multidoc,
        ),
        (
            "nolima-semantic-needle",
            "NoLiMa-style semantic needle retrieval",
            "This example asks the harness to find a semantically described fact with low lexical overlap.",
            "NoLiMa semantic needle retrieval",
            "semantic retrieval",
            ["NoLiMa"],
            "The question intentionally uses different surface words than the target passage.",
            nolima_semantic_needle,
        ),
        (
            "helmet-citation-rag",
            "HELMET-style citation RAG",
            "This example asks the harness to answer a release question with explicit supporting citations.",
            "HELMET RAG and citation tasks",
            "citation-grounded RAG",
            ["HELMET"],
            "The final answer must reconcile policy, release, and rollout-risk documents.",
            helmet_citation_rag,
        ),
        (
            "bright-rag-reasoning",
            "BRIGHT-style technical RAG reasoning",
            "This example asks the harness to solve a multi-hop technical support question.",
            "BRIGHT-style long-context RAG reasoning",
            "multi-hop technical RAG",
            ["BRIGHT", "RAGBench", "Long2RAG"],
            "The final answer depends on matching an incident, deployment matrix, and patch note.",
            bright_rag_reasoning,
        ),
    ]

    for (
        example_name,
        title,
        summary,
        source,
        task_family,
        inspired_by,
        design_note,
        builder,
    ) in examples:
        question, context, expected_answer = builder()
        write_example(
            example_name,
            title,
            summary,
            source,
            task_family,
            inspired_by,
            design_note,
            question,
            context,
            expected_answer,
        )


if __name__ == "__main__":
    main()
