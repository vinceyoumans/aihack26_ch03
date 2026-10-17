# Challenge 1: The Bad Week Radar

**Can a wearable spot a rough patch before the person says anything?**

People going through cancer treatment often wear a fitness tracker. Some weeks are rough: less energy, worse
sleep, fewer steps. Coaches usually hear about it days later, at the next call. Build something that warns
early, for the patient, their coach, or both.

## What's in this folder

| File | One row is | Size | Use it for |
|---|---|---|---|
| `fatigue_study_daily.csv` | one person, one day | 9,317 rows, 296 people | **Real data.** What a bad day looks like on the wrist. |
| `lifesnaps_daily.csv` | one person, one day | 7,410 rows, 71 people | **Real data.** What normal looks like for one person over months. |
| `lifesnaps_hourly.csv` | one person, one hour | 159,508 rows | Same people as above, hour by hour. |
| `synthetic_data/` | made-up patients in treatment | 3,000 people, 465,466 days | **Big and made up.** Build and demo your app here. A few patterns are hidden on purpose. |

**Rule of thumb:** build on the big made-up data, and back up any claim about real people with the real data.

## Load it in 30 seconds

```python
import pandas as pd
df = pd.read_csv("fatigue_study_daily.csv")
df.groupby("day_rating")[["steps", "sleep_minutes", "resting_heart_rate"]].mean()

days = pd.read_csv("synthetic_data/synthetic_daily.csv")
days[days.events.str.contains("ER visit", na=False)].head()
```

```javascript
// In a web app: 250 made-up patients, one object each, with every day inside
const { patients } = await (await fetch("synthetic_data/synthetic_250_patients.json")).json();
console.log(patients[0].treatment_plan, patients[0].days.length);
```

---

## 1. `fatigue_study_daily.csv` (real)

296 US adults followed for about a month (August to October 2020). About half live with lupus or Sjögren's
syndrome, autoimmune illnesses where fatigue comes and goes in flares, a lot like it does during chemo. The
rest are people without those conditions. Every morning they answered a few questions about the day before.
Once a week they filled in FACIT-Fatigue, the fatigue questionnaire used in cancer trials. 207 of them also
wore a Fitbit.

| Column | What it means |
|---|---|
| `person_id` | Anonymous ID |
| `date` | The day the row describes |
| `health_group` | lupus, Sjögren's syndrome, lupus and Sjögren's, or no autoimmune condition |
| `age`, `sex` | From the sign-up survey |
| `study_week` | Week 1 to 4 of the study |
| `day_rating` | How was the day? good, average or bad |
| `bad_day` | 1 if rated bad, 0 if good or average |
| `tired_0_to_4` | How tired: 0 not at all, 4 very much |
| `pain_0_to_4` | Pain: 0 none, 4 extreme |
| `low_mood_0_to_4` | Felt down or depressed: 0 not at all, 4 extremely |
| `trouble_with_usual_activities_0_to_4` | Trouble doing normal things like work or chores: 0 none, 4 extreme |
| `needed_daytime_sleep_0_to_2` | Needed to sleep during the day: 0 no, 1 once, 2 more than once |
| `weekly_fatigue_score_0_to_52` | FACIT-Fatigue for the week this day belongs to. **Lower means more tired.** |
| `steps` | Steps that day |
| `minutes_brisk_activity` | Minutes walking briskly (over 100 steps a minute) |
| `minutes_light_activity` | Minutes of light movement (50 to 100 steps a minute) |
| `minutes_sitting_still` | Minutes with fewer than 50 steps |
| `resting_heart_rate` | Fitbit's resting heart rate (beats per minute) |
| `avg_heart_rate_awake`, `avg_heart_rate_asleep` | Average heart rate while awake and while asleep |
| `sleep_minutes` | Minutes asleep |
| `time_in_bed_minutes` | Minutes in bed for the main sleep |
| `sleep_efficiency_pct` | Share of time in bed actually asleep |
| `bedtime_hour` | When the main sleep started. 22.5 = 10:30 pm, 24.5 = 12:30 am |
| `naps` | Number of naps |
| `minutes_fitbit_worn` | Minutes the Fitbit was on and recording (low = they took it off) |

**Good to know**
- The daily answers describe the *day before* they were given. We already moved them to the day they describe.
- The weekly fatigue score is copied onto all seven days it covers. Don't use it to predict that same week.
- 89 people answered the surveys but had no Fitbit. Their Fitbit columns are blank.
- Source: Rao C, et al. *Association of digital measures and self-reported fatigue.* Frontiers in Digital Health, 2023.
  Data from Evidation, Novartis and Roche on Zenodo: https://zenodo.org/records/8018238

## 2. `lifesnaps_daily.csv` and `lifesnaps_hourly.csv` (real)

71 adults in Greece, Cyprus, Italy and Sweden wearing a Fitbit Sense for about four months (2021 to 2022),
with short mood check-ins on their phone. Good for learning each person's normal and spotting when it changes.

**Who:** `person_id`, `date` (and `hour` in the hourly file), `age_group` (under or over 30), `sex`, `bmi_group`

**Activity:** `steps`, `distance_m`, `calories_burned`, `minutes_light_activity`, `minutes_moderate_activity`,
`minutes_vigorous_activity`, `minutes_sedentary`, `step_goal`. Hourly file: minutes in each Fitbit heart rate zone.

**Heart and body:** `resting_heart_rate`, `avg_heart_rate`, `hrv_rmssd_ms` (heart rate variability; higher usually
means better recovered), `heart_rate_deep_sleep`, `blood_oxygen_spo2_pct`, `breathing_rate_sleep`,
`skin_temp_night_c`, `skin_temp_change_c` (change from the person's usual), `skin_conductance_avg` (a sweat-based
stress signal), `fitbit_stress_score_0_to_100` (Fitbit's score; **higher means less stress**),
`fitness_vo2max_estimate`, `did_mindfulness_session`

**Sleep:** `sleep_minutes`, `minutes_to_fall_asleep`, `minutes_awake_at_night`, `sleep_efficiency_pct`,
`sleep_deep_share`, `sleep_rem_share`, `sleep_light_share`

**Mood check-ins:** `felt_happy`, `felt_alert`, `felt_neutral`, `felt_rested_relaxed`, `felt_sad`,
`felt_tense_anxious`, `felt_tired`. 1 = picked that mood, 0 = checked in but didn't pick it,
blank = no check-in. `any_mood_reported` = 1 if they checked in that day (about 2,300 of the 7,410 days).

**Where they were:** `at_home`, `at_home_office`, `at_work_or_school`, `outdoors`, `at_gym`, `in_transit`,
`at_entertainment`, `somewhere_else` (same 1 / 0 / blank logic)

**Good to know**
- Not every sensor works every day, so expect blanks. Heart rate variability, oxygen and the stress score are the patchiest.
- Source: Yfantidou S, et al. *LifeSnaps, a 4-month multi-modal dataset.* Scientific Data, 2022. CC BY 4.0.
  https://zenodo.org/records/7229547

## 3. `synthetic_data/` (made up, for building)

**Nobody in these files is real.** 3,000 made-up people in Miami going through cancer treatment while wearing a
Fitbit, about 465,000 days in all. Each person's usual steps, heart rate, heart rate variability, sleep and day
to day wobble were drawn from the real data above. On top of that we layered treatment: 17 cancer types and 10
kinds of treatment (IV chemo every 3 weeks, every 2 weeks or weekly, chemo pills, targeted pills,
immunotherapy, radiation, chemo plus radiation, hormone therapy), with side effects, infections, ER visits,
hospital stays, dose delays, skipped check-ins and days the watch stays in a drawer. People differ on
purpose: some sail through, some get worse each round, some slowly stop using the app.

A few patterns are hidden in here on purpose. Finding them shows your method works. They are our design,
though, so any claim about real patients needs the real data.

| File | One row is | Size |
|---|---|---|
| `synthetic_patients.csv` | one person | 3,000 people |
| `synthetic_daily.csv` | one person, one day | 465,466 rows |
| `synthetic_hourly_250_patients.csv` | one person, one hour | 717,456 rows for 250 of the people |
| `synthetic_250_patients.json` | one person, with all their days inside | the same 250 people, ready for a web or phone app |

**`synthetic_patients.csv`**

| Column | What it means |
|---|---|
| `patient_id` | P00001 and so on. Matches the other files. |
| `age`, `sex`, `race_ethnicity`, `bmi` | Who they are. The mix looks like South Florida. |
| `preferred_language` | English, Spanish or Haitian Creole. Diary notes come in this language most of the time. |
| `cancer_type`, `stage` | Type of cancer and stage I to IV |
| `treatment_plan` | For example "IV chemo every 21 days" or "Radiation, weekdays for 5 weeks" |
| `device` | Fitbit tracker or Fitbit smartwatch. Only the smartwatch reads skin temperature. |
| `lives_alone`, `miles_to_clinic` | yes or no; distance from home to the clinic |
| `enrolled_on`, `days_in_program` | Start date and how many days of data they have |

**`synthetic_daily.csv`**

| Column | What it means |
|---|---|
| `patient_id`, `date`, `day_in_program` | Who and when |
| `treatment_phase` | before treatment, on treatment, after treatment |
| `treatment_today` | 1 on an infusion day, a radiation session, or the first day of a pill cycle |
| `treatment_cycle`, `day_of_cycle` | For treatments given in rounds. Day 1 is the dose day. Blank for radiation and daily pills. |
| `steps`, `active_minutes`, `resting_heart_rate`, `hrv_rmssd_ms` | Daily Fitbit numbers. Blank means the watch wasn't worn. |
| `sleep_minutes`, `sleep_efficiency_pct`, `times_woke_up` | Last night's sleep |
| `skin_temp_change_c` | Change in skin temperature from the person's usual, in degrees C. Smartwatch only. |
| `minutes_watch_worn` | Out of 1,440. Low means they took it off. |
| `daily_feeling_1_to_5` | Quick daily check-in: 1 awful, 5 great. Blank means they skipped it. |
| `weekly_fatigue_0_to_4` ... `weekly_sadness_0_to_4` | Weekly symptom check-in: fatigue, nausea, pain, appetite loss, diarrhea, trouble sleeping, anxiety, sadness. 0 none, 4 very severe. Only on the check-in day. |
| `coaching_call` | completed, missed or rescheduled. Every two weeks. |
| `events` | ER visit, admitted to hospital, discharged home, dose delayed one week, dose reduced, treatment paused, treatment completed |
| `diary_note` | A short note the person typed in the app that day, if any. English, Spanish or Haitian Creole. |

**`synthetic_hourly_250_patients.csv`:** `patient_id`, `date`, `hour` (0 to 23), `steps`, `avg_heart_rate`,
`asleep` (1 or 0). Hours with the watch off are left out.

**`synthetic_250_patients.json`:** `{"about": ..., "patients": [ { every column from the patients file, "days": [ one object per day with the daily columns ] } ]}`

**Good to know**
- "Rough patch" isn't a column. You decide what counts: a low daily feeling, an ER visit, high symptom scores, or your own mix.
- Missing data is realistic on purpose. People skip check-ins more on bad days and take the watch off in hospital.
- Made with a fixed random seed, so everyone has the same files.

## Credit

Put the two real datasets on your sources slide with the citations above. The synthetic data was made by the
Crane Lab at Sylvester Comprehensive Cancer Center for this hackathon.
