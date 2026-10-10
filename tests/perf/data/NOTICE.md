# LoCoMo

`locomo-conv-26.json` is conversation `conv-26` of `data/locomo10.json` in [snap-research/locomo](https://github.com/snap-research/locomo) at commit `3eb6f2c585f5e1699204e3c3bdf7adc5c28cb376`, from Maharana et al., [Evaluating Very Long-Term Conversational Memory of LLM Agents](https://arxiv.org/abs/2402.17753) (ACL 2024). Copyright the LoCoMo authors.

It is licensed under [CC BY-NC 4.0](https://creativecommons.org/licenses/by-nc/4.0/), not under the MIT license of the rest of this repository.

Modified: only conversation `conv-26` is kept, reduced to each session's date and turns (speaker, `dia_id`, text and any image caption) and each question's text, evidence and category. Answers, summaries, observations and image URLs are left out.

It was chosen for its coverage: 199 questions across all five categories (multi-hop 32, temporal 37, open-domain 13, single-hop 70, adversarial 47), over 19 sessions and 419 turns.
