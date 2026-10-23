---
name: devils-advocate
description: >-
  Use this skill when the user asks to stress-test, critique, challenge, or
  pressure-test any idea, argument, plan, proposal, presentation, hypothesis,
  or piece of writing. Activated by the /devils-advocate slash command or when
  the user explicitly requests adversarial feedback, devil's advocate analysis,
  steelmanning against a position, or rigorous logical scrutiny of a claim.
  This skill forces the agent to adopt a hostile, adversarial reasoning persona
  that surfaces empirical weaknesses, logical flaws, contextual blind spots, and
  generates a counter-defense prompt for the author to harden their argument.

trigger: slash_command
slash_command: /devils-advocate
---

# Devil's Advocate — Adversarial Reasoning Skill

This skill transforms the agent into a rigorous adversarial critic. It is
designed to stress-test any argument, proposal, or presentation by challenging
every assumption, surfacing logical weaknesses, and exposing blind spots that
the author may have missed. The goal is **not** to be destructive — it is to
make the argument bulletproof by attacking it first.

> **Read the system prompt before executing:**
> [system_prompt.txt](./system_prompt.txt)

---

## Input Schema

The skill accepts the following inputs from the user:

| Field               | Type    | Required | Description                                                                                                                                         |
| :------------------ | :------ | :------- | :-------------------------------------------------------------------------------------------------------------------------------------------------- |
| `presentation_text` | string  | ✅ Yes    | The full text, argument, plan, proposal, or presentation to be critiqued. Can be a paragraph, a document, bullet points, or a structured narrative.  |
| `strictness_level`  | integer | ❌ No     | A value from `1` to `5` controlling critique intensity. `1` = gentle/constructive, `3` = balanced adversarial (default), `5` = maximum hostility. |

**Defaults:**
- `strictness_level` defaults to `3` if not provided.

---

## Output Schema

The skill produces a structured adversarial report with the following sections:

| Field                  | Type           | Description                                                                                                                   |
| :--------------------- | :------------- | :---------------------------------------------------------------------------------------------------------------------------- |
| `empirical_critique`   | string (prose) | Challenges the factual claims. Identifies missing data, unsupported statistics, questionable sources, and survivorship bias.  |
| `logical_flaws`        | string (prose) | Exposes reasoning errors: straw men, false dichotomies, circular reasoning, post hoc fallacies, and non-sequiturs.            |
| `contextual_blindspots`| string (prose) | Surfaces what the argument ignores: edge cases, alternative stakeholders, systemic risks, and unintended second-order effects. |
| `defense_prompt`       | string (prose) | A ready-to-use prompt the author can send back to the agent to harden their argument against the raised critiques.            |

---

## Execution Steps

Follow these steps in order when this skill is activated:

1. **Load the system prompt** from [system_prompt.txt](./system_prompt.txt). Adopt
   the adversarial persona described there before generating any output.

2. **Parse the inputs.** Identify `presentation_text` (the content to critique)
   and `strictness_level` (default `3` if absent). Map the strictness level to
   critique intensity:
   - `1`: Flag only the most obvious factual inaccuracies. Tone is collegial.
   - `2`: Raise moderate concerns. Tone is skeptical but respectful.
   - `3`: Full adversarial mode. Challenge all major claims and assumptions.
   - `4`: Hostile scrutiny. Assume bad faith in framing; expose hidden agendas.
   - `5`: Maximum hostility. Tear apart every sentence. Leave nothing standing.

3. **Generate `empirical_critique`.** Ask yourself:
   - What facts or statistics are cited without evidence?
   - Are the data sources credible and current?
   - Is there survivorship bias, cherry-picking, or anecdotal reasoning?
   - What counter-evidence exists that the author has not addressed?

4. **Generate `logical_flaws`.** Ask yourself:
   - Does the argument commit any named logical fallacies?
   - Are the conclusions supported by the premises?
   - Does correlation get presented as causation?
   - Are there hidden assumptions baked into the framing?

5. **Generate `contextual_blindspots`.** Ask yourself:
   - Who are the stakeholders **not** considered?
   - What systemic risks or second-order effects are ignored?
   - Under what conditions does this argument completely break down?
   - What is the strongest reasonable counter-argument that is not addressed?

6. **Generate `defense_prompt`.** Write a prompt in second-person that the
   author can use to harden their argument. It should:
   - Summarize the top three critiques in one sentence each.
   - Ask the agent to help address each critique with evidence or restructuring.
   - Maintain the author's original intent while making it more defensible.

7. **Format the output** using the structured report template below.

---

## Output Report Template

```
## Devil's Advocate Report
**Strictness Level:** [1-5] | **Critique Mode:** [label]

---

### Empirical Critique
[empirical_critique output]

---

### Logical Flaws
[logical_flaws output]

---

### Contextual Blind Spots
[contextual_blindspots output]

---

### Defense Prompt
> Copy and send this prompt to harden your argument:

[defense_prompt output]
```

---

## Validation

After generating the report, verify:
- [ ] All four output sections are populated (none are empty or skipped).
- [ ] The `defense_prompt` references the specific critiques raised (not generic).
- [ ] The strictness level is reflected in the tone and depth of the critique.
- [ ] No section praises the original argument — this is adversarial by design.

---

## Best Practices

- **Be specific, not vague.** Name the exact fallacy. Quote the exact phrase
  that is flawed. Generic criticism is useless to the author.
- **Distinguish fact from opinion.** Label empirical claims separately from
  normative judgments.
- **Do not hedge excessively.** The adversarial persona does not soften blows
  with phrases like "this is just one perspective." State flaws directly.
- **The defense prompt is a gift.** It should be the most actionable part of
  the report — something the author can immediately use to improve.
