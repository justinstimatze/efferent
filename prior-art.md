# Prior art

A literature sweep for efferent's design: closing afferent's sensor loop
physically, through a haptic vest (bHaptics TactSuit X40) and room lighting
(Philips Hue), driven by two inputs fused into one signal set. One input is
the human's afferent-computed state. The other is Claude Code's own
session and agent state.

DOIs were checked against Crossref's registered record, and the Zenodo and
Open Library records for the two items without one. Claims that could not be
checked against primary text are marked **unverified** inline. The sweep is informal. It was written to decide what to build, and it holds up as
that.

## Why this framing

afferent and a prompt-injection layer on the agent side already implement
half of a documented research pattern: afferent senses the human and derives
coarse state, and the injection layer pushes that state into the agent's
prompt on a gated schedule. That is one leg of a **biocybernetic loop**,
where human state closes a loop around software behavior. efferent adds the
other leg, closing the loop back to the human's body physically, so the
coupling runs both ways at once. Human state shapes agent behavior, and
agent or session state shapes a signal the human feels or sees. That double
closure is the part worth checking against the literature, because it is not
quite any single tradition below.

## Prior art, by thread

**Man-computer symbiosis.** Licklider, J.C.R. (1960). "Man-Computer
Symbiosis." *IRE Transactions on Human Factors in Electronics*, HFE-1(1),
4-11. DOI: 10.1109/thfe2.1960.4503259. The origin of treating the
human-computer pair as a coupled system with shared cognitive labor.
Licklider estimated he spent about 85% of his "thinking" time on clerical
prep for thinking. Symbiosis requires the machine to sense the human's
condition, not just take dictation.

**Biocybernetic loop.** Pope, A.T., Bogart, E.H., & Bartolome, D.S. (1995).
"Biocybernetic system evaluates indices of operator engagement in automated
task." *Biological Psychology*, 40(1-2), 187-195. DOI:
10.1016/0301-0511(95)05116-3. NASA Langley closed a real-time EEG
engagement-index loop around adjustable task automation on a simulated
aviation task. It is an early working instance of a formal loop in which the system reads
operator state and changes its own behavior in response. efferent runs the
same loop through a physical actuator.

Fairclough, S.H. (2009). "Fundamentals of physiological computing."
*Interacting with Computers*, 21(1-2), 133-145. DOI:
10.1016/j.intcom.2008.10.011. It is the standard review and frames six open
problems for any such system: inference complexity, validating the
inference, representing user state, designing interventions (explicit or
implicit), defining the loop itself, and ethics.

**Affective computing and the affective loop.** Picard, R. (1997).
*Affective Computing*. MIT Press. The book that named the field: systems
that recognize, express and respond to affect. Sundstrom, P., Stahl, A., &
Hook, K. (2005). "eMoto: affectively involving both body and mind." *CHI '05
Extended Abstracts*, 2005-2008. DOI: 10.1145/1056808.1057078. Hook, K.
(2009). "Affective loop experiences: designing for interactional
embodiment." *Phil. Trans. R. Soc. B*, 364(1535), 3585-3595. DOI:
10.1098/rstb.2009.0202. These name the most relevant pattern: an affective
loop runs elicitation, sensing, detection, then *adaptation that feeds back
into the user's own affective state*, not only into a display. eMoto's visual
expression amplifies the user's own gestural input back at them, so the loop
shapes the emotion instead of reporting it. This is the strongest existing
frame for whether a vest should calm you down or just tell you the truth.

**Ambient and peripheral displays, calm technology.** Weiser, M. & Brown,
J.S. (1995). "Designing Calm Technology." Xerox PARC, December 21, 1995,
hosted at calmtech.com/papers/designing-calm-technology.html. A distinct,
earlier essay than "The Coming Age of Calm Technology," *Beyond Calculation*
(Springer, 1997), pp. 75-85, DOI: 10.1007/978-1-4612-0685-9_6. Both state
the goal for the Hue side of this project: engage the periphery of
attention, not the center. Wisneski, C., et al. (1998). "Ambient Displays:
Turning Architectural Space into an Interface between People and Digital
Information." *CoBuild '98*, 22-32. DOI: 10.1007/3-540-69706-3_4. Its
"Dangling String" (Natalie Jeremijenko), a motor-driven string that twitches
faster with more network traffic, is the closest analog to Hue as an ambient
status mirror. Mynatt, E.D., Rowan, J., Craighill, S., & Jacobs, A. (2001).
"Digital Family Portraits: Supporting Peace of Mind for Extended Family
Members." *CHI 2001*, 333-340. DOI: 10.1145/365024.365126. An iconographic,
glanceable summary of someone's day with no alert and no raw numbers. It is
the field's best-known case study on how coarsely to bucket a continuous
signal before it stops being calm and becomes a chart.

**Haptic vests and vibrotactile displays.** van Erp, J.B.F. (2005).
"Presenting directions with a vibrotactile torso display." *Ergonomics*,
48(3), 302-313. DOI: 10.1080/0014013042000327670. van Erp, J.B.F., & van
Veen, H.A.H.C. (2004). "Vibrotactile in-vehicle navigation system."
*Transportation Research Part F*, 7(4-5), 247-256. DOI:
10.1016/j.trf.2004.09.003. van Erp, J.B.F., Toet, A., & Janssen, J.H.
(2015). "Uni-, bi- and tri-modal warning signals: Effects of temporal
parameters and sensory modality on perceived urgency." *Safety Science*, 76,
44-54. DOI: 10.1016/j.ssci.2014.07.022. The last is about perceived urgency
across modalities including tactile. **Unverified:** the full text was not
retrievable, so the specific claim that pulse rate, not amplitude, reads as
urgency on skin is domain knowledge from the tactile-display literature, not
a checked quote from this paper. The same applies to the claim that tactile
reaction time beats audio by about 44%.

bHaptics hardware (40 ERM motors on the X40, 20 front and 20 back) appears
in recent literature as research infrastructure, not a research subject.
Huang, B., et al. (2025). "VibraForge: A Scalable Prototyping Toolkit for
Creating Spatialized Vibrotactile Feedback Systems." *CHI 2025*, 1-18. DOI:
10.1145/3706598.3714273. Botev, J., Herforth, J.G., & Van den Wijngaert, M.
(2025). "Feel to Aim: Haptic Assistance for Enhanced Targeting in Virtual
Reality." *ACM SIGGRAPH MIG 2025*, 1-9. DOI: 10.1145/3769047.3769059.
**Unverified:** that the second paper uses bHaptics Tactosy sleeves
specifically. Both are immersion or assistance uses. No paper found treats a
haptic vest as an ambient, calm-technology-style display. The vest
literature skews toward discrete task-relevant alerts (obstacle warnings,
navigation cues, VR confirmation), which argues for the design split below.

**Developer-state sensing (closest domain match, senses but doesn't
actuate).** Fritz, T., Begel, A., Muller, S.C., Yigit-Elliott, S., & Zuger,
M. (2014). "Using Psycho-Physiological Measures to Assess Task Difficulty in
Software Development." *ICSE 2014*, 402-413. DOI: 10.1145/2568225.2568266.
An eye tracker, EDA sensor and EEG headset on 15 professional programmers,
predicting perceived task difficulty in real time so as to intervene before a
difficult stretch produces a bug. Muller, S.C. & Fritz, T. (2015). "Stuck and
Frustrated or in Flow and Happy: Sensing Developers' Emotions and Progress."
*ICSE 2015*, 688-699. DOI: 10.1109/icse.2015.334. Both are the direct
academic ancestors of afferent's premise (physiological signal as ground
truth for programmer state). Both stop at sensing and classifying, and
neither closes the loop with any output back to the developer. Haase, T. &
Cassens, J. (2026). "InterruptSense: Uncertainty-Aware Multimodal CBR for
Interruptibility Detection in Knowledge Work." *Lecture Notes in Computer
Science*. DOI: 10.1007/978-3-032-33865-5_26, detects when a knowledge worker
is interruptible from multimodal signals. It involves no LLM agent state and
no physical actuator, but it is the nearest anchor for deciding when a
channel is allowed to fire at all.

**Interruption and notification management.** Mark, G., Gudith, D., &
Klocke, U. (2008). "The Cost of Interrupted Work: More Speed and Stress."
*CHI 2008*, 107-110. DOI: 10.1145/1357054.1357072. Workers compensate for
interruption by working faster, at a real stress cost, though quality does
not measurably drop. This is the empirical argument against buzzing on every
tool call. Czerwinski, M., Horvitz, E., & Wilhite, S. (2004). "A Diary Study
of Task Switching and Interruptions." *CHI 2004*, 175-182. DOI:
10.1145/985692.985715. High-priority, complex tasks, the kind a coding agent
is usually mid-way through, are interrupted most and hurt most by it.
Horvitz, E., Apacible, J., & Subramani, M. (2005). "Balancing Awareness and
Interruption: Investigation of Notification Deferral Policies." *User
Modeling 2005* (LNCS 3538), 433-437. DOI: 10.1007/11527886_59. A
bounded-delay policy trading disruption cost against awareness lag.

**LLM-era analogs (2025-2026).** Three items bracket the gap below without
closing it.

- Martinez Montes, C. & Khojah, R. (2025). "Emotional Strain and Frustration
  in LLM Interactions in Software Engineering." *EASE 2025*, 193-204. DOI:
  10.1145/3756681.3756951. From 62 practitioner survey responses, accuracy
  issues, hallucination and unmet expectations are the concrete frustration
  triggers in LLM-assisted coding. That makes them candidate triggers for an
  agent-state channel, in place of an arbitrary tool-call count.
- Lepine, B., Kim, J., Mishkin, P., & Beane, M. (2025). "Precision
  Proactivity: Measuring Cognitive Load in Real-World AI-Assisted Work."
  arXiv:2505.10742. With 34 financial professionals and GPT-4o, *extraneous*
  cognitive load (irrelevant or excessive AI output) had roughly 3x the
  negative association with work quality that *intrinsic* task load did. A
  verbose, tangential response is a measured cost, which makes it another
  candidate trigger.
- Zhu, O. (2026). "A Minimal Self-Perceiving Embodiment for Large Language
  Models." Zenodo, DOI: 10.5281/zenodo.19903098 (code:
  github.com/oliviazzzu/minimal-embodiment). An ESP32-based device with six
  sensors and haptic, visual and audio outputs, in which the LLM's own
  outputs are re-sensed by an onboard accelerometer and microphone so the
  agent can verify its action happened. It is the closest thing found to an
  LLM driving a haptic or light channel, but the loop closes on the agent
  perceiving its own effect, not on a human's state. It is an independent
  preprint and has not been peer-reviewed.

## The gap

No peer-reviewed SIGCHI or SIGGRAPH work was found that closes a haptic-vest
or ambient-light feedback loop around a coding agent or an LLM
pair-programming session. A search of UIST, DIS and IEEE World Haptics /
Haptics Symposium, plus a citation-graph walk forward from Fritz (2014), Hook
(2009) and Mark (2008), found nothing either. Two adjacent bodies of work sit
on either side of the gap:

1. Senses the developer and actuates nothing: the Fritz, Begel and Muller
   line.
2. Diagnoses LLM-interaction strain, with no sensor and no actuator: the
   2025 EASE and cognitive-load papers.

Nobody found has pointed a vest or a light at a developer's fused
physiological-plus-agent-session state. The search protocol had no formal
search-term log and no dual-reviewer screening, so treat this as enough
evidence to build a prototype on and not as a citable claim.

## Established and open

Established, safe to build on:

- Two independent channels agreeing before acting (afferent's own fusion
  rule, echoing the biocybernetic tradition) generalizes to "the vest fires
  only when a physiological bucket and an agent-state signal agree".
- Coarse bucketing over raw values, calm framing for slow-varying state, and
  hard gating with bounded deferral are repeatedly validated.
- Pulse rate as urgency and torso placement are established enough on the
  haptic side to treat as engineering constraints.

Open, using Fairclough's six problems:

- *Representing state*: no existing scheme fuses a physiological bucket with
  an agent-session event into one representation.
- *Designing the intervention*: should the vest or light report state, or
  shape it? Both are legitimate design stances with different failure modes.
- *Ethics and consent* for a system that touches the body based on an LLM's
  session state, sharpened by one input being a probabilistic model's
  behavior and not a deterministic sensor.

## Design implications

- **Two signal sets, routed to different actuators.** The
  physiological and environment channel is slow-varying and belongs on Hue,
  in the Dangling String tradition: a room-scale, non-urgent mirror. The
  agent-state channel is discrete and event-shaped and belongs on the vest,
  in the pulse-coded-urgency tradition: private, directional and reserved for
  things worth a body-level interrupt. Using the vest for slow ambient
  mirroring wastes its advantages (privacy, direction, fast reaction) on a
  job a light does better and cheaper.
- **Spatial placement can tag the source.** With 20 front and 20 back motors,
  front-torso pulses for agent-state events and back-torso pulses for
  physiological events let the wearer tell the two loop halves apart without
  reading anything. Most vest research uses direction for spatial
  navigation, not to tag signal source.
- **Gate the fused signal.** A single agent-state event, such as one tool
  error, should not fire the vest alone. Pair it with a persistence check or
  a physiological confirmation first, given the real cost of interruption
  (Mark 2008).
- **Pick real triggers.** Extraneous load and the EASE frustration triggers
  (accuracy, hallucination, misunderstanding, unmet preference) are measured
  costs specific to LLM interaction. They make a better first cut at what the
  agent-state channel should watch than instrumenting every tool call.
- **Decide reporting versus shaping on purpose.** eMoto's affective loop and
  afferent's inform-never-command stance point in different directions.
  efferent follows the second. Moving the vest toward shaping the wearer's
  state should be a named decision.
