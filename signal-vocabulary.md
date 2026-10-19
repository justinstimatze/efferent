# Signal vocabulary

efferent drives three surfaces (Hue lights, a haptic vest, a Launchpad) from
a few kinds of input. This document records the small, fixed set of
primitives per surface and how the signals compose from them, so a new event
reuses an existing primitive instead of inventing a new behavior.

## Why composition

Brewster & Brown, "Non-visual information display using tactons" (CHI '04
Extended Abstracts, DOI `10.1145/985921.985936`), is the method copied here:
build a small set of orthogonal parameters (rhythm, location, intensity) and
construct a vocabulary from their combinations.

McCrickard & Chewar, "Attuning notification design to user goals and
attention costs" (CACM 2003, DOI `10.1145/636772.636800`), names the failure
a bespoke signal runs into: notifications carry an interruption cost, a
reaction cost and a comprehension cost. A raw event count fails on
comprehension. It reports that something happened without saying what, and
the reader pays later by hunting through a log.

When two independent channels agree on a reading, recognition becomes
corroboration instead of a decode step. Van Erp's crossmodal warning-signal
work (see `prior-art.md`) is the applied version of this. The three surfaces
should express the same event consistently where they overlap.

## Related literature

- Wisneski et al., "Ambient Displays: Turning Architectural Space into an
  Interface between People and Digital Information" (CoBuild 1998, DOI
  `10.1007/3-540-69706-3_4`), coined "ambient display". Its running example,
  a string that twitches harder with network traffic, is the ancestor of
  Machine's Breath: an environmental object carrying system activity
  peripherally, never demanding acknowledgment.
- Pousman & Stasko, "A Taxonomy of Ambient Information Systems" (AVI 2006,
  DOI `10.1145/1133265.1133277`), separates notification level (how hard a
  display may interrupt) from representational fidelity (how precisely it
  encodes the value). An ambient display trades fidelity away on purpose, so
  it should be judged on notification level.
- Mankoff et al., "Heuristic Evaluation of Ambient Displays" (CHI 2003, DOI
  `10.1145/642611.642642`), gives usability heuristics written for this
  display class.
- Fairclough, "Fundamentals of Physiological Computing" (Interacting with
  Computers, 2009, DOI `10.1016/j.intcom.2008.10.011`), is the academic home
  for a system that reads physiology and closes a loop back to the wearer,
  which is what afferent and efferent do together.
- Wiener, "Cybernetics, or Control and Communication in the Animal and the
  Machine" (MIT Press reissue DOI `10.7551/mitpress/11810.001.0001`), is the
  root of the sense-and-actuate framing. The names afferent and efferent
  are nerve-signal-direction terms.

Design touchstones, cited as lineage and not as research: the red/amber/green
severity convention (near-universal, so almost no training cost) and LCARS,
the Star Trek console language, for a fixed small palette composed with
panel position on a surface you look at and touch.

## Three attention registers

| Surface | Register | You engage it by |
|---|---|---|
| Hue | Deep periphery | Glancing, from across the room, optional |
| Vest | Body periphery | Feeling it, worn; it can be tuned out but not looked away from |
| Launchpad | Focal | Looking at and touching it, deliberately |

Weiser & Brown's "Designing Calm Technology" (see `prior-art.md`) is the
frame: center versus periphery of attention. Hue and vest carry things that
should reach you unasked. The Launchpad carries detail you go looking for,
so a buzz never has to carry information it structurally can't.

## Hue

One signal: **Machine's Breath**, a brightness rhythm around a fixed
soft-white baseline. The rate follows the agent's turn phase from
`agenthook`: about 3 s per breath while the agent is working, about 12 s
while it awaits the user. It is a mixer channel (`machine_breath.go`) and
starts muted.

Color is not used. A color mirror of CO2 and the brightness rhythm were both
computed from the same value, so in practice they were not independent
parameters, and the two could not be told apart by eye. "The air is bad,
open a window" is better said in words than shown on a light.

Not built: a green flow-state halo that would fire once on a good
transition.

## Vest

Three independent dimensions:

- **Location**: back (motors 10-29) carries body and environment state;
  front (motors 0-9 and 30-39) is currently silent.
- **Rhythm**: a sharp discrete pulse is an edge-triggered alert; a smooth
  sustained ramp is entrainment or presence.
- **Intensity**: routine signals stay at or below `0x5` of `0xF`, enforced
  by the mixer's comfort ceiling, with a hard ceiling of `0xA` on every
  write. `0xF` is mildly uncomfortable to wear (`vest-protocol.md`).

Signals:

- **Fatigue pulse**: one sharp pulse on the back when afferent's fatigue
  factor crosses its threshold. It fires on the rising edge only, and not
  while the vest is reconnecting.
- **Fleet breath**: a slow spatial fill rising up the back, whose rate and
  peak follow the number of Claude Code sessions working across the fleet.
  It is a mixer channel, opt-in per strip, and the MIDImix dials set the
  fill's vertical center, left/right balance and edge sharpness. Earlier
  hardcoded versions that ran continuously, including one that tapped the
  wearer's own heart rate, did not hold up worn for a session
  (`haptic-mixing-prior-art.md`). Echoing the wearer's own rhythm back reads
  as a mirror whatever the intensity.

Tool errors are not a haptic trigger. An error count carries no way to check
what the error was, and many have self-corrected by the time the buzz fires.

### Coregulation grounding

The vest as a felt presence, not only a data channel, rests on several
findings:

- Feldman, "Parent-infant synchrony and the construction of shared timing"
  (J Child Psychol Psychiatry 2007, DOI `10.1111/j.1469-7610.2006.01701.x`),
  documents measurable physiological coupling between caregiver and infant.
- Coan, Schaefer & Davidson, "Lending a Hand: Social Regulation of the
  Neural Response to Threat" (Psychological Science 2006, DOI
  `10.1111/j.1467-9280.2006.01832.x`), found that hand-holding downregulates
  the brain's threat response with no information content required.
- Loken et al., "Coding of pleasant touch by unmyelinated afferents in
  humans" (Nature Neuroscience 2009, DOI `10.1038/nn.2312`), identified
  C-tactile afferents tuned to slow, light touch. A discrete motor tap is not
  the slow-stroking stimulus they respond to, so this is directional
  grounding for "soft, sustained contact reads differently from an alert".
- Azevedo et al., "The calming effect of a new wearable device during the
  anticipation of public speech" (Scientific Reports 2017, DOI
  `10.1038/s41598-017-02274-2`), is the closest artificial precedent: a
  wrist device delivering an artificial heartbeat-like rhythm reduced
  anticipatory anxiety, and the rhythm was not phase-locked to the wearer's
  own pulse.
- Wada & Shibata, "Living With Seal Robots" (IEEE Trans. Robotics 2007, DOI
  `10.1109/TRO.2007.906261`), measured benefit from an artificial companion,
  which supports a worn object mediating a coregulation-shaped signal.

## Launchpad

The Launchpad is the detail surface, with two panels.

- **Fleet panel** (`fleet.go`, `fleet_panel.go`): one grid pad per live
  Claude Code session, positioned to mirror the tmux cockpit and colored by
  phase (working, waiting on you, stuck). Phase comes from each transcript's
  `stop_reason`, not from file growth. A core gauge shows fleet-wide
  activity, pressing a pad jumps the cockpit to that session, and a staged
  boot sequence plays at startup.
- **Negotiation panel** (`negotiation.go`): the top control row holds a hold
  toggle, four self-report pads (calm, ok, frayed, done) and a landed/missed
  turn rating. Presses are written to files under the state directory. Nothing
  reads them yet.

## Composition

| Signal | Hue | Vest | Launchpad |
|---|---|---|---|
| Fatigue threshold | none | back, sharp pulse, once on crossing | none |
| Machine's Breath (agent phase) | brightness breathing, two-state, mixer channel, muted by default | none | none |
| Fleet breath (working-session count) | none | back, spatial fill, rate and peak scale with the count, mixer channel, muted by default | none |
| Fleet health | none | none | grid, one pad per session, phase-colored, plus core gauge, press to jump |
| Self-report and turn rating | none | none | control row, records to files |

## Open questions

- Whether the Launchpad's grid layout should mirror the vest's front/back
  split visually.
- Whether the vest's front channel gets a signal, such as a sweep tied to a
  message arriving from another agent session.
- Whether Machine's Breath on the Launchpad grid, as a brightness echo,
  would add useful corroboration.
