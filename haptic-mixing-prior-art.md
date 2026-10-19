# Haptic mixing and mixing-console prior art

Extends `prior-art.md` and `signal-vocabulary.md`. It addresses two
engineering questions for the vest mixer (fleet breath plus further channels,
blended and gain-controlled from an Akai MIDImix): what happens when two
channels want the same motor, and whether a physical mixing-console metaphor
has precedent as a tuning interface for an ambient or haptic system. A third
question is whether "a signal that fades itself on identical repetition" has
a name in the literature.

Source status is marked inline. **Verified** means a primary source was
checked directly (Crossref's registered record for anything with a DOI, the
Zenodo API, Open Library, Europe PMC, or the actual PDF or page).
**Metadata-only** means the bibliographic record was verified but the body
could not be fetched, with the reason named. **Inferred** means reasoned from
something checked. **Gap** means searched and found nothing.

---

## Thread A — how simultaneous vibrotactile channels actually combine

### A1. Two nearby simultaneous actuators do not read as two things. They read as one.

This is the key finding, and it is old, replicated, and modelled.

**von Békésy, G. (1958). "Funneling in the Nervous System and its Role in
Loudness and Sensation Intensity on the Skin." *Journal of the Acoustical
Society of America*, 30(5), 399-412. DOI `10.1121/1.1909626`** (verified
against Crossref). The origin of the funneling illusion on skin.

**Alles, D.S. (1970). "Information Transmission by Phantom Sensations."
*IEEE Transactions on Man-Machine Systems*, 11(1), 85-91. DOI
`10.1109/tmms.1970.299967`** (verified). Proposed linear and logarithmic
relations between the physical actuators' intensities and the resulting
phantom.

**Israr, A. & Poupyrev, I. (2011). "Tactile Brush: Drawing on Skin with a
Tactile Grid Display." *Proc. CHI 2011*, 2019-2028. DOI
`10.1145/1978942.1979235`** (verified against Crossref, and full text read
directly — hosted openly by the authors' own institution at
`la.disneyresearch.com`). Three things from that text matter here:

1. **Funneling and "phantom sensation" are the same phenomenon.** Quoted
   directly: "Phantom tactile sensation is also known as the funneling
   illusion and should not be confused with the phantom limb illusion
   discussed elsewhere."

2. **The combination rule is energy summation, which is neither linear sum
   nor max.** Quoted directly: "Following the energy summation model in the
   Pacinian channels [26], we propose that the intensity of a phantom
   actuator Av is calculated as follows: Av^2 = A1^2 + A2^2, Eq. 1, where
   A1 and A2 are intensities of physical actuators," with stated
   assumptions that the two actuators run at the same frequency and sit at
   locations of comparable skin sensitivity — "a reasonable assumption for
   closely-spaced actuators." The paper explicitly notes this "departs
   significantly from the currently used linear and logarithmic models
   proposed by Alles." Reference [26] is **Makous, J.C., Friedman, R.M. &
   Vierck, C.J. (1995). "A critical band filter in touch." *Journal of
   Neuroscience*, 15(4), 2808-2818. DOI `10.1523/jneurosci.15-04-02808.1995`**
   (verified) — the source of the Pacinian energy-summation claim.

3. **This was measured on the back, at 63mm spacing.** Quoted: "We used a
   4x3 equally-spaced rectangular grid of C-2 tactors with inter-tactor
   spacing of 63 mm (2.5"), placed in a finely cut sheet of foam padding
   and glued to the back of a wooden chair." Same body surface as the
   vest's back panel, at a spacing almost certainly *wider* than a
   10-motor vertical column implies. **Inferred:** funneling is therefore
   the expected default behaviour for vertically adjacent vest motors, not
   a marginal risk.

**Park, G. & Choi, S. (2018). "Tactile Information Transmission by 2D
Stationary Phantom Sensations." *Proc. CHI 2018*, 1-12. DOI
`10.1145/3173574.3173832`** (verified metadata; ACM DL 403, body unread).
The follow-on: treating the phantom itself as the information-carrying
unit in 2D.

**Direct answer to the sum-vs-max question:** neither, and the literature
says so with a validated model. For actuators close enough to funnel, the
perceptually correct combination is root-sum-square —
`sqrt(A1^2 + A2^2)` — and the percept's *location* also moves, to a point
between the two motors set by their intensity ratio. `max` under-drives
the resulting single percept; `sum` over-drives it; both get the location
wrong the same way, because neither models that a location is being
synthesised at all.

### A2. Simultaneity does not just blur identity — it destroys it

**Yeganeh, N., Makarov, I., Kristjánsson, Á. & Unnthorsson, R. (2023).
"Discrimination Accuracy of Sequential Versus Simultaneous Vibrotactile
Stimulation on the Forearm." *Applied Sciences*, 14(1), 43. DOI
`10.3390/app14010043`** (verified against Crossref; MDPI's site 403s
automated fetches via Akamai, so the body is unread — quoted numbers are
from the publisher-deposited Crossref abstract). A 2x3 voice-coil array at
100 Hz on the forearm:

> "Recognition accuracy was significantly higher for sequential (93.24%)
> than for simultaneous presentation (26.15%). Patterns using 2-3
> actuators were recognized more accurately than those using 4-5
> actuators."

**Caveat:** pattern-*identification* on the forearm, not two continuous
ambient channels on a torso array. Bounds the pessimistic case rather than
describing efferent's exact case.

**Chase, E.D.Z., Israr, A., Schepmann, M.S., O'Malley, M.K. &
Hartcher-O'Brien, J. (2026). "Information Transfer on the Wrist:
Vibrotactile Signal Characteristics from Single to Multiple Tactors."
*ACM Transactions on Human-Robot Interaction*, 15(5), 1-39. DOI
`10.1145/3822501`** (verified against Crossref; ACM DL 403, body unread;
abstract publisher-deposited). Quoted:

> "Information transfer ranged from 0.75 bits/stimulus for multi-tactor
> patterns to 2.28 bits/stimulus for single-tactor testing across all
> participants (2.56 for experienced participants); masking and typing
> yielded 1.64 and 1.85 bits/stimulus, respectively."

Multi-tactor patterns carried under a third the information of
single-tactor cues — a 2026 paper from the same lineage as Tactile Brush
(shared author), arriving at the pessimistic result fifteen years later
with a formal information measure.

**Wersényi, G. (2022). "Perception Accuracy of a Multi-Channel Tactile
Feedback System for Assistive Technology." *Sensors*, 22(22), 8962. DOI
`10.3390/s22228962`, PMC9695395** (verified, and full text read directly
via Europe PMC's full-text XML — MDPI's own site 403s). 27 subjects,
wrists/arms/ankles/forehead, 2/4/8-channel patterns. Quoted:

> "Results indicate that 2 and 4 channels could be used simultaneously
> with high accuracy… For optimal performance, signal length should
> exceed 500 ms."

> "Due to good sensitivity on the wrists, any further channels should be
> attenuated, otherwise it can mask other channels… It is recommended to
> set vibration attenuation individually…"

> "…differences in the amplitudes are more important than differences in
> the vibration frequencies. If the 235 Hz on one wrist is not compensated
> (damped accordingly) in the amplitude to match the 185 Hz or 300 Hz
> signals' amplitudes, the latter signals can be completely masked."

Three takeaways: **2-4 concurrent channels is the empirical ceiling**, and
that's for widely distributed body sites, not a dense torso panel;
**per-channel attenuation, set individually, is the documented fix** for a
channel that's too strong — almost word for word an argument for the
MIDImix; and **amplitude mismatch is the masking mechanism**, with frequency
secondary, and amplitude is exactly the variable a fader controls.

**Gallace, A., Tan, H.Z. & Spence, C. (2006). "Numerosity Judgments for
Tactile Stimuli Distributed over the Body Surface." *Perception*, 35(2),
247-266. DOI `10.1068/p5380`** (verified metadata; body unread) and the
same authors' **"Can Tactile Stimuli Be Subitised? An Unresolved
Controversy within the Literature on Numerosity Judgments." *Perception*,
37(1), 2008. DOI `10.1068/p5767`** (verified metadata). Cited for the
shape of the constraint (how many simultaneous tactile things a person can
even count), not a specific numeral — the second title is a fair warning
the exact number is contested.

### A3. Masking is not local. It reaches across the body.

The part that most changes how the vest's 40 motors should be thought
about.

**Craig, J.C. (1968). "Vibrotactile spatial summation." *Perception &
Psychophysics*, 4(6), 351-354. DOI `10.3758/BF03209532`** (verified
metadata; body unread).

**Craig, J.C. (1976). "Attenuation of vibrotactile spatial summation."
*Sensory Processes*** (verified via Europe PMC, PMID 1029076, abstract
read directly — no DOI in EPMC's record). Quoted:

> "Low-frequency stimuli showed no spatial summation, whereas
> high-frequency stimuli did. In the presence of a masker, spatial
> summation was reduced or eliminated for high-frequency stimuli…
> Additional measurements with the left thenar eminence as the test site
> showed that spatial summation could be attenuated by a masker placed on
> a **contralateral** body site."

A vibration on the *other side of the body* changed the measured
threshold. Forty motors on a torso are one skin, with lateral
interactions, not forty independent channels.

Supporting, verified metadata only (bodies not fetched): **Gescheider,
G.A., Bolanowski, S.J. & Verrillo, R.T. (1989). "Vibrotactile masking:
Effects of stimulus onset asynchrony and stimulus frequency." *JASA*,
85(5), 2059-2064. DOI `10.1121/1.397858`**; **Craig, J.C. (1995).
"Vibrotactile masking: The role of response competition." *Perception &
Psychophysics*, 57(8), 1190-1200. DOI `10.3758/bf03208375`**; **Verrillo,
R.T. (1963). "Effect of Contactor Area on the Vibrotactile Threshold."
*JASA*, 35(12), 1962-1966. DOI `10.1121/1.1918868`** — the classic
demonstration that threshold falls as contact area grows, i.e. spatial
summation at the receptor level.

**Ng, K.K.W., Birznieks, I. & Vickery, R.M. (2026). "Vibrotactile
intensity perception: Predominant influence of afferents recruited remote
from stimulus site." *The Journal of Physiology*. DOI `10.1113/jp290236`**
(verified via Europe PMC, PMID 42329899, abstract read directly). Their
modelling indicates perceived intensity is dominated by the *population*
response including afferents well away from the contact point. **Caveat:**
hand, and the manipulation is pulses-per-burst rather than two concurrent
channels — cited as mechanism, not a direct torso result.

**Cheng, C.H., Eden, J., Oetomo, D. & Tan, Y. (2025). "Exploring
Interference between Concurrent Skin Stretches." arXiv:2503.21044**
(verified against arXiv's own metadata/abstract page, fetched directly).
30 participants:

> "We observed that when participants experience simultaneous skin
> stretch stimuli, a masking effect occurs which deteriorates perception
> performance in the collocated skin stretch configurations… multi-channel
> skin stretch feedback designs should avoid locating modules in close
> proximity."

**Flag:** skin stretch, not vibration — corroborating evidence for the
general shape (concurrent channels interfere, co-location aggravates it),
not a vibration-specific result.

### A4. Torso placement and the saltation window

**Cholewiak, R.W., Brill, J.C. & Schwab, A. (2004). "Vibrotactile
localization on the abdomen: Effects of place and space." *Perception &
Psychophysics*, 66(6), 970-987. DOI `10.3758/bf03194989`** (verified
metadata; body unread). The torso-specific localization study — anatomical
anchor points (navel, spine) and how well people can place a buzz on the
trunk.

**Geldard, F.A. & Sherrick, C.E. (1972). "The Cutaneous 'Rabbit': A
Perceptual Illusion." *Science*, 178(4057), 178-179. DOI
`10.1126/science.178.4057.178`** (verified metadata; body unread). Sensory
saltation. **Inferred only:** saltation requires rapid-succession
taps at nearby sites within a short window; a *continuous* hum plus a
*discrete* pulse close in space and time is the configuration where it
would be expected — a mislocated tap, felt somewhere neither channel put
it. No study found of saltation specifically between a continuous
background and a discrete foreground signal. Treat as a hypothesis to
test, not a prediction to trust.

### A5. Does the literature predict the Hue collapse, or something cleaner?

An earlier Hue experiment, in which two nominally orthogonal parameters (color
and brightness rhythm) could not be told apart, had an implementation
confound: both were computed from the same value. It therefore never tested
the orthogonality claim in Tactons.

On the vest there is no such ambiguity. Funneling, energy summation,
cross-body masking, and numerosity limits are all measured perceptual
effects. Two continuous channels driving overlapping or vertically-adjacent
back-panel motors will merge into a single percept whose location sits
between them and whose intensity follows RSS. Not a bug to engineer
around — what the skin does.

The sharper point to carry into the alphabet: **Brewster & Brown's
orthogonal parameters are not equally orthogonal.** Rhythm and location
have real evidence behind them as separable. *Intensity* is the parameter
that does not stay orthogonal to anything, because intensity is precisely
the variable funneling, masking, and spatial summation all operate on. A
design that composes two channels by adding intensity at a shared motor is
composing along the one axis the perceptual machinery is built to merge.
The Hue finding generalises, and the generalisation is "the
intensity axis is the one that entangles," not "Tactons is wrong."

### A6. Where the literature does not answer the question

- **Nothing found measures two independent, continuous, *ambient*
  vibrotactile channels blended on a worn array over hours, in a
  non-identification task.** Every source above is short-duration
  threshold, discrimination, or pattern-identification work. efferent's
  actual case — a hum you're not attending to, plus something else, for a
  working day — is not in this corpus.
- **No vendor documentation resolves the overlap rule.** Fetched
  `docs.bhaptics.com/sdk/unity/references/library` directly (HTTP 200,
  read). It documents multi-coordinate rendering where "all actuators
  around these coordinates in the array will activate simultaneously… not
  sequentially," but says nothing about what happens when two
  independently-playing events both address the same motor.
- **The one obvious industrial artifact could not be read.** US patent
  12,450,991, "Haptics mixing for concurrent haptic events" — the USPTO
  print endpoint returns a 19-page PDF with no text layer (`pdftotext`
  extracted 3 bytes); Google Patents returned a 1.4 KB shell. Title/number
  as returned by the USPTO endpoint; contents unread, claims unverified.
  Cheapest next step if the sum-vs-max question stays live: OCR it.

---

## Thread B — the mixing console as a control surface for a non-audio system

### B1. There is a direct hit, and it is a good one

**McNamara, T., McCormack, J. & Llano, M.T. (2025). "Mixer Metaphors:
audio interfaces for non-musical applications." *Proc. NIME '25*,
Canberra, 24-27 June 2025. Zenodo DOI `10.5281/zenodo.15699645`** (verified
against the Zenodo API, and the full PDF read directly from
`nime.org/proceedings/2025/nime2025_47.pdf`).

The device: the **Memetic Mixer** — knobs, faders, presets, filters and
effects lifted from analogue synths and mixing consoles, controlling an
LLM. Quoted from the abstract:

> "In this paper we reverse this tradition to ask, can interfaces
> developed for music be successfully appropriated to non-musical
> applications? … Our results show that the use of audio-like controls
> afforded more immediate, direct and embodied control over the LLM…"

Two key findings, both quoted from the fetched PDF:

**(a) Users invented the metaphor themselves, unprompted, before it
existed.** During a focus group on the pre-Mixer device, a participant
suggested manipulating the LLM with tactile "knobs and sliders" — a knob
to "dial up or down" traits like "snarkiness" or "verbosity." The authors:
"At this stage of the study, the Memetic Mixer did not exist. This comment
was completely unprompted."

**(b) The metaphor carries the interaction with no instruction cost.**
"They were not inhibited by the lack of instructions for these new control
parameters as the audio interface metaphor made the interaction style
immediately apparent." One participant: "to be clear, I would not read the
manual!"

The paper also offers a reusable design-question toolkit (F.1-F.6 for
faders, K.1-K.6 for knobs, P.1-P.4 for presets) for applying mixer
paradigms elsewhere. F.2 ("What parameters are being mixed together to
create a whole?"), F.3 ("Which parameters could be assigned an independent
fader?") and F.4 ("What parameters could I group together with a single
fader?") map onto eight MIDImix strips almost mechanically.

**Caveats:** creative-expression domain, artists as participants, small
sample, one week per condition, no haptics. Right precedent for "does this
metaphor transfer off music," not evidence about ambient-display comfort
tuning specifically.

### B2. Level-as-a-notification-primitive: the auditory-display lineage

- **Gaver, W.W., Smith, R.B. & O'Shea, T. (1991). "Effective sounds in
  complex systems: the ARKola simulation." *Proc. CHI '91*, 85-90. DOI
  `10.1145/108844.108857`** (verified metadata; body unread — ACM DL 403s
  even at the "GOLD open access" PDF URL Semantic Scholar reports).
  **Closest structural analogue found in the entire sweep** — many
  concurrent continuous status streams sharing one perceptual bus — and
  its body was not read. Highest-value follow-up read.
- **Sawhney, N. & Schmandt, C. (2000). "Nomadic radio…" *ACM TOCHI*, 7(3),
  353-383. DOI `10.1145/355324.355327`** (verified metadata; body unread).
  Scaleable auditory notification — presentation intensity as a continuum
  driven by estimated importance, not binary fire/don't-fire. A software
  gain control on a notification.
- **Mynatt, E.D., Back, M., Want, R., Baer, M. & Ellis, J.B. (1998).
  "Designing audio aura." *Proc. CHI '98*, 566-573. DOI
  `10.1145/274644.274720`** (verified metadata; body unread).
- **Butz, A. & Jung, R. (2005). "Seamless user notification in ambient
  soundscapes." *Proc. IUI '05*, 320-322. DOI `10.1145/1040830.1040914`**
  (verified metadata; body unread).

### B3. Physical controllers as generic parameter surfaces

- **Greenberg, S. & Fitchett, C. (2001). "Phidgets…" *Proc. UIST '01*,
  209-218. DOI `10.1145/502348.502388`** (verified metadata; body
  unread). Ancestor of "a slider is just an input device you can bind to
  anything."
- **Ishii, H. et al. (1998). "ambientROOM…" *CHI '98 Conference Summary*.
  DOI `10.1145/286498.286652`** (verified metadata, full author list
  confirmed; body unread). Nearest physical-interface-for-ambient-displays
  precedent found — but about binding sources to channels, not live
  per-channel gain.
- Haptic authoring already uses a DAW/timeline metaphor at *design* time:
  **Schneider & MacLean (2016), Macaron, DOI `10.1109/haptics.2016.7463155`**;
  **van Oosterhout, Bruns & Hoggan (2020), Feelix, DOI
  `10.1145/3382507.3418819`** (+ 2022 follow-up, DOI
  `10.1145/3547522.3558900`); **Schneider, Israr & MacLean (2015), Tactile
  Animation, DOI `10.1145/2807442.2807470`** (all verified metadata, bodies
  unread) — the last decouples authored haptic objects from the physical
  grid they render onto, structurally the same problem as blending
  independent channels onto shared motors. **None is a live per-channel
  gain surface** — the metaphor is present in haptics; console-at-runtime
  use is not.
- **Hurley, J., Drummond, J. & Bongers, A.J. (2026). "Haptic Interfaces
  for Accessible Audio Mixing." *IEEE Trans. Haptics*. DOI
  `10.1109/toh.2026.3716643`** (verified metadata; body unread). Runs the
  arrow the other way — haptics as *display* for a mixing task, not a
  mixing console as *control* for haptics. A clean inversion, not the
  precedent needed.
- **Non-academic precedent is routine.** Verified directly by fetching
  `lightjams.com/midiDMX.html` (HTTP 200, read): a commercial product
  mapping arbitrary MIDI controllers onto DMX stage lighting, named
  touring users. Equivalent use in OBS (faders bound to audio-source gain)
  is widely reported in that project's own forums. Vendor/practitioner
  material, not research — evidence the MIDI plumbing is a solved problem
  in practice. It is not a study of anything.
- **Unfollowed lead:** Rebecca Fiebrink's Wekinator (arbitrary physical
  controllers mapped to arbitrary output parameters via interactive ML)
  surfaced repeatedly and looks directly relevant. No citation verified
  flagged as a lead, not presented as checked.

### B4. The actual gap on this thread

No academic framing found of ambient-display *calibration* or
comfort-tuning as a mixing/gain problem with a physical console. Not a
shallow miss: "ambient display calibration" returns ambient-*light*
photometry, a different sense of the word, and existing notification
personalization work is about *when/whether* to fire and says nothing on
continuously-adjustable per-channel intensity set by hand. The
MIDImix-as-tuning-console appears genuinely unoccupied territory. Strongest
available evidence it'll work: Mixer Metaphors — the same metaphor
transfer, tested, in an adjacent non-musical domain, with users reaching
for it unprompted.

---

## Thread C — "the system gets bored of itself": does that idea have a name?

Short answer: the phenomenon has a well-established name and a formal
model. The specific design pattern does not appear to have one.

### C1. The phenomenon is habituation, and it is thoroughly characterised

- **Rankin, C.H. et al. (2009). "Habituation revisited…" *Neurobiology of
  Learning and Memory*, 92(2), 135-138. DOI `10.1016/j.nlm.2008.09.012`**
  (verified metadata, full 14-author list confirmed; body unread). The
  canonical modern consensus statement of habituation's defining
  characteristics.
- **Groves, P.M. & Thompson, R.F. (1970). "Habituation: A dual-process
  theory." *Psychological Review*, 77(5), 419-450. DOI
  `10.1037/h0029810`** (verified metadata; body unread). Repeated
  stimulation drives *two* opposed processes — habituation and
  sensitisation — whose sum determines the observed response.
- **Sokolov, Y.N. (1963). *Perception and the Conditioned Reflex*** —
  verified as a real 1963 monograph via Open Library (`/works/OL4543630W`);
  a 1966 *American Journal of Psychology* review carries DOI
  `10.2307/1419876` (that's the review, not the book). The orienting-response
  source.
- **Gershman, S.J. (2024). "Habituation as optimal filtering." *iScience*.
  DOI `10.1016/j.isci.2024.110523`** (verified metadata; body unread;
  preprint at `10.31234/osf.io/ystw2`). Closest to a *principled*
  formulation of "fade what's stopped being informative" — habituation as
  optimal filtering rather than fatigue. Worth reading if a self-fading signal
  needs a defensible mechanism instead of a heuristic decay.
- **Berlyne, D.E. (1970). "Novelty, complexity, and hedonic value."
  *Perception & Psychophysics*, 8. DOI `10.3758/bf03212593`** and
  **Berlyne, D.E. (1960). *Conflict, Arousal, and Curiosity*. DOI
  `10.1037/11164-000`** (both verified metadata; bodies unread). The
  inverted-U behind "there's an optimum amount of novelty."
- **Lewandowska, A., Dziśko, M. & Jankowski, J. (2022). "…contrast on
  habituation and sensitisation effects in peripheral areas of graphical
  user interfaces." *Scientific Reports*. DOI `10.1038/s41598-022-16284-2`**
  (verified metadata; open access on nature.com, not fetched —
  cheap follow-up). Closest HCI-side empirical anchor for habituation
  specifically in the periphery of a display, which is the register
  efferent's signals occupy.

**Unverified:** the corollary that weak repeated stimuli tend to habituate
while strong ones can sensitise instead is standard secondary-source
framing. Neither primary text was readable (both paywalled). Treat it as a
hypothesis with verified anchors.

### C2. The engineering pattern has a name too, just not in HCI

- **Marsland, S., Nehmzow, U. & Shapiro, J. (2005). "On-line novelty
  detection for autonomous mobile robots." *Robotics and Autonomous
  Systems*, 51(2-3), 191-206. DOI `10.1016/j.robot.2004.10.006`** (verified
  metadata; body unread). Same mechanism, opposite framing — a robot
  suppressing the familiar to detect the novel. (Marsland is also a
  co-author on the Rankin 2009 consensus paper above.)
- **Schmidhuber, J. (2010). "Formal Theory of Creativity, Fun, and
  Intrinsic Motivation (1990-2010)." *IEEE Trans. Autonomous Mental
  Development*, 2(3), 230-247. DOI `10.1109/TAMD.2010.2056368`** (verified
  metadata; body unread). Boredom as the decline of compression progress —
  a formal theory of a system getting bored.

### C3. Musicolour

Pask's Musicolour is the earliest working artifact in this family.

**Bird, J. & Di Paolo, E. (2008). "Gordon Pask and His Maverick Machines,"
ch. 8 in Husbands, Holland & Wheeler (eds.), *The Mechanical Mind in
History*. MIT Press. Chapter DOI `10.7551/mitpress/9780262083775.003.0008`**
(verified against Crossref, and the chapter PDF read directly from the
second author's university page at `users.sussex.ac.uk`). Quoted:

> "Pask built the first Musicolour system, a sound-actuated interactive
> light show, in 1953."

> "The output from each filter is averaged over a short period, rectified,
> and passed through an associated adaptive threshold device… These
> devices adapt their threshold to the mean value of the input,
> habituating to repetitive input, for example a continuous sound in a
> particular pitch band, and outputting 0."

> "If the input to Musicolour is repetitive, it habituates and adjusts its
> filter parameter values in an attempt to generate more variety in the
> light patterns. If there is no input, the system becomes increasingly
> sensitive to any sound in the environment and a gain control prevents
> this from disrupting the system too much."

Musicolour habituated to *its input* (the performer's repetition) and
responded by *increasing the variety of its own output*. It did not soften
its own output on repetition of that output. A signal that fades after
firing identically N times is a **self-referential** variant, a cousin of
Musicolour and not an instance of it. The two point in opposite directions:
Musicolour got louder and stranger when things went stale, and a self-fading
signal gets quieter.

A further behaviour worth borrowing is Musicolour's **rising
sensitivity in the absence of input, bounded by a gain control** — an
automatic-gain-control architecture, in 1953, on the problem the
MIDImix addresses.

### C4. Do staircase / PEST / QUEST connect to this? Not really, and the mismatch matters

All verified metadata, bodies unread:

- **Cornsweet, T.N. (1962). "The Staircase-Method in Psychophysics."
  *American Journal of Psychology*, 75(3), 485. DOI `10.2307/1419876`**
- **Taylor, M.M. & Creelman, C.D. (1967). "PEST…" *JASA*, 41(4A), 782-787.
  DOI `10.1121/1.1910407`**
- **Watson, A.B. & Pelli, D.G. (1983). "QUEST…" *Perception &
  Psychophysics*, 33(2), 113-120. DOI `10.3758/BF03202828`**
- **Leek, M.R. (2001). "Adaptive procedures in psychophysical research."
  *Perception & Psychophysics*, 63(8), 1279-1292. DOI
  `10.3758/bf03194543`** — the standard review in exactly this sub-field
  (also cited by Tactile Brush).

No documented connection found between these procedures and
self-fading-on-repetition. Two conceptual mismatches worth naming before
building on a staircase:

1. **These procedures converge on a detection *threshold*.** A stimulus
   parked at threshold is, by construction, detected only some fraction of
   the time. For a presence channel that may be acceptable. For anything
   that must be reliably felt, it is wrong. A rule that steps down on no
   response and up on "didn't notice" is a threshold-seeking staircase, and
   threshold is not the target: comfortable-and-reliably-felt is a
   different quantity.
2. **These procedures assume a stationary observer.** Habituation makes
   the observer non-stationary — the threshold moves as a function of the
   very presentations the staircase is making. A staircase run against a
   habituating observer chases its own tail downward. Leek's review is the
   place to check whether the standard toolkit has a remedy; its body was not
   read. Highest-value follow-up read on this thread.

**Direct answer:** *habituation* names the phenomenon, *habituation-based
novelty filtering* / *adaptive thresholding* names the mechanism,
*Musicolour* is the earliest working artifact — and, as far as this sweep
found, **nothing names the HCI design pattern** of an ambient signal that
softens itself on identical repetition. The corner looks unoccupied,
consistent with the gap noted in `prior-art.md`.

---

## Synthesis: established vs. open

**Established enough to treat as engineering constraints:**

- Two simultaneous vibrotactile sources close together on the torso
  produce **one** percept, located between them, intensity following
  **root-sum-square**. Modelled and validated, on the back, at 63mm
  spacing.
- Sequential beats simultaneous by a large margin for anything that must
  be identified (93% vs 26% in one direct comparison); adding actuators
  *reduces* information transfer.
- Masking is amplitude-dominated, crosses distance, demonstrated
  **contralaterally**. Forty motors are not forty channels.
- ~2-4 concurrent channels is the observed practical ceiling, for widely
  distributed body sites.
- Habituation is the likely explanation for presence signals that stopped
  working after repeated exposure: well characterised, formally modelled,
  with documented recovery behaviour.
- The mixing-console metaphor transfers to non-musical parameter spaces at
  ~zero instruction cost, with users reaching for it unprompted.

**Genuinely open — no literature found either way:**

- The perceptual behaviour of two *continuous ambient* vibrotactile
  channels blended on a worn array over hours, non-identification task.
  efferent's exact case; not in this corpus.
- Ambient-display calibration/comfort-tuning framed as a live-gain/console
  problem.
- Self-fading-on-identical-repetition as a named HCI design pattern.
- How commercial haptic middleware resolves two concurrent events on one
  actuator.

---

## Design implications

**Blend rule:**

1. **Neither sum nor max — root-sum-square, then clamp**:
   `sqrt(A1^2 + A2^2)`, per Israr & Poupyrev's validated model. The only
   one of the three with a psychophysical model behind it.
2. **RSS also happens to land inside this project's own comfort band.**
   Inferred arithmetic, not literature: two channels each at `0x4` give
   `max` = `0x4` (under-drives, channels become indistinguishable), `sum`
   = `0x8` (the pacer peak already retired as "pretty strong"), RSS ≈
   `0x5.7` → `0x5`/`0x6` (inside the band live testing already blessed).
   Coincidence, not derivation — worth knowing anyway.
3. **Treat needing a blend rule at all as a smell.** The literature's real
   answer is *don't overlap*. Separate channels in **space** (front/back
   is already a location primitive — promote it from convention to
   allocation *constraint*), **time** (sequential wins by a lot), or
   **kind** (one continuous, one rare-and-discrete).
4. **Budget by numerosity.** Motor count is the wrong unit. Plan for 2-3 concurrent
   channels as the ceiling on a dense torso panel; any fourth needs a
   positive argument. 40 motors are bandwidth for rendering one or two
   well-shaped percepts, not forty facts — which matches
   live use: the usable band on this vest is one soft ramp.
5. **Mind the saltation window** if a discrete pulse fires near a
   continuous hum on the same zone. Inferred only: rapid
   succession at nearby sites is the saltation configuration. Test for it
   rather than assume it away.

**The MIDImix:**

6. **Build it. The strongest argument is iteration speed, and calibration
   theory comes second.** Tuning failures show up only in live use, on an
   iteration loop measured in hours to days. A fader collapses that to
   seconds.
7. **Apply gain as a pre-blend ceiling multiplier.** A fader move should change that channel's
   contribution to the merged percept, and a post-blend trim would only
   scale the already-merged result. Matches Wersényi's
   measured recommendation to attenuate per-channel individually.
8. **Solo is more valuable than mute.** "Which channel is that?" is the
   recurring comprehension failure for any multi-channel signal (see
   McCrickard & Chewar in `signal-vocabulary.md`). Solo answers it
   physically, in one press, with no drill-down surface.
9. **Log every fader move with a timestamp and the full signal state at
   that moment.** This yields a labeled tuning record from a
   control there is an independent reason to touch, which avoids relying on
   anyone pressing dedicated feedback pads. A fader move is a self-report
   with a magnitude attached.
10. **Steal the Mixer Metaphors design questions for the strip layout.**
    F.2 (what's being mixed to make a whole), F.3 (what deserves an
    independent fader), F.4 (what should group under one fader) are
    directly the "eight strips, how many channels" questions, from a paper
    that tested them.

**Self-fading:**

11. **Implement as an explicit habituation model with recovery, not a
    monotonic decay.** Spontaneous recovery is one of habituation's
    defining characteristics; decay-only fades to nothing and stays there.
12. **Don't drive the gain with a threshold-seeking staircase.** Staircases
    converge on "just barely detected," not the target, and assume a
    stationary observer, which habituation violates. If a staircase is
    used at all, target a comfort criterion with a floor, not a detection
    criterion.
13. **Consider Musicolour's actual move as an alternative to fading.** It
    responded to staleness by increasing variety and raising its own
    sensitivity in silence, bounded by a gain control. For a
    presence signal, "vary it slightly when stale" may preserve presence
    better than "mute it when stale", and has a 1953 existence proof.

---

## What wasn't found

- No study of two or more **continuous ambient** vibrotactile channels
  blended on a worn array over a long session, non-identification task.
  The entire corpus is short-duration threshold/discrimination/
  identification work. efferent's exact case isn't covered — the
  perceptual literature constrains the design without settling it.
- No academic framing of **ambient-display calibration/comfort-tuning as a
  mixing/gain problem**, with or without a physical console.
- No **name** for the self-fading-on-identical-repetition design pattern.
  The phenomenon (habituation) and mechanism (habituation-based novelty
  filtering, adaptive thresholding) are named; the design pattern isn't.
- No **documented combination rule** for overlapping haptic events on a
  shared actuator, academically or in the bHaptics SDK reference that was
  fetched and read.
- No study of **saltation between a continuous background and a discrete
  foreground** vibrotactile signal.

---

## Verification and method note

**Read directly:** Israr & Poupyrev's *Tactile Brush* PDF; McNamara,
McCormack & Llano's *Mixer Metaphors* PDF (nime.org); Bird & Di Paolo's
Musicolour chapter PDF (author's university page); Wersényi 2022 full text
(Europe PMC XML); Craig 1976 and Ng et al. 2026 abstracts (Europe PMC);
Cheng et al. 2025 abstract (arXiv); the Zenodo API record for Mixer
Metaphors; Open Library for Sokolov; the bHaptics Unity library reference;
and the Lightjams MIDI-to-DMX page.

**Crossref metadata only, body unread:** Yeganeh et al. 2023 and Chase et
al. 2026 (publisher sites returned 403; the quoted abstracts are
publisher-deposited to Crossref). Elsevier and Springer bodies were not
fetched. Every other DOI had title, authors, venue, year and pagination
checked against Crossref, and "metadata-only" means that is all that was
done.

**Not read:** US patent 12,450,991 (the USPTO print endpoint returns an
image-only PDF, so only title and number are confirmed). Wekinator
(Fiebrink) is a lead from search and is deliberately left uncited.

**Recalled identifiers need checking.** Two DOIs written from memory resolved
cleanly to unrelated papers (Nomadic Radio and the Cholewiak abdomen study)
and were corrected by bibliographic search. A DOI that resolves to the wrong
paper is a worse failure than one that 404s, because the lookup appears to
succeed.

**Method limits.** Keyword and citation search across Crossref, Europe PMC,
arXiv, Zenodo, Open Library and general web search, with direct fetches
wherever a source allowed. It is not a systematic review: no search-term log,
no dual screening, no exhaustive snowballing. The gap claims in Thread B4
and Thread C mean "not found in this sweep", which is enough to build a
prototype on and not enough to publish as a novelty claim. Highest-value
unread items, in order: ARKola (CHI '91), Leek's adaptive-procedures review,
and Lewandowska's peripheral-GUI habituation paper (open access on
nature.com).
