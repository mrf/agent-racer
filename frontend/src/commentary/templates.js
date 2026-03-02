/**
 * Commentary templates organized by trigger type.
 * Each trigger has an array of template strings.
 * Use {name} for session name substitution.
 */

export const TEMPLATES = {
  session_start: [
    'And {name} pulls onto the track! A new challenger approaches!',
    '{name} fires up the engine and joins the race!',
    'We have a newcomer! {name} enters the fray!',
    'The crowd goes wild as {name} rolls onto the starting grid!',
    'Ladies and gentlemen, {name} has entered the race!',
    '{name} revs their engine and takes position on the track!',
  ],

  overtake: [
    '{name} blasts past {other} with incredible speed!',
    'What a move! {name} overtakes {other}!',
    '{name} makes their move — leaving {other} in the dust!',
    'A daring overtake by {name}, pushing past {other}!',
    '{name} surges ahead of {other}! The crowd is on their feet!',
  ],

  context_50: [
    '{name} is halfway through their context window! Still plenty of runway.',
    'The 50% mark for {name} — burning tokens at a steady clip.',
    '{name} crosses the midway point. Half the context consumed!',
    'Halfway there! {name} has used 50% of their context budget.',
  ],

  context_90: [
    'WARNING: {name} is at 90% context! Running out of room!',
    '{name} is pushing the limits — 90% context used! Can they finish in time?',
    'The red zone for {name}! Only 10% of context remaining!',
    'Danger zone! {name} is burning through tokens fast — 90% consumed!',
  ],

  error: [
    '{name} spins out! An error on the track!',
    'Oh no! {name} hits the wall! That is going to leave a mark!',
    'Trouble for {name} — they have crashed out of the race!',
    '{name} is down! A catastrophic error takes them out!',
    'And {name} goes up in smoke! A dramatic exit from the race!',
  ],

  completion: [
    '{name} crosses the finish line! What a performance!',
    'CHECKERED FLAG for {name}! They have done it!',
    '{name} completes their task with style! Victory lap incoming!',
    'And {name} is DONE! The crowd erupts!',
    'A flawless finish by {name}! Absolutely brilliant!',
  ],

  idle_long: [
    '{name} seems to be taking a nap in the pit lane...',
    'Has anyone checked on {name}? Very quiet over there.',
    '{name} is idling... conserving energy or lost in thought?',
    'Not much action from {name}. The pit crew is getting restless.',
  ],

  high_burn: [
    '{name} is burning tokens like there is no tomorrow!',
    'Incredible pace from {name} — the token counter is spinning!',
    '{name} has the pedal to the metal! Massive token consumption!',
    'Look at that burn rate! {name} is absolutely flying!',
  ],

  compaction: [
    '{name} hits a compaction event! Context optimized, back in action!',
    'Memory refresh for {name} — compaction complete, pushing forward!',
    '{name} just got a context tune-up! Compaction keeps them in the race!',
  ],

  subagent_spawn: [
    '{name} deploys a sub-agent! Teamwork makes the dream work!',
    'A hamster joins the fleet! {name} calls in reinforcements!',
    '{name} spawns a helper — divide and conquer!',
    'Backup has arrived! {name} sends out a sub-agent for support!',
  ],

  tool_use: [
    '{name} reaches for the toolbox! Tools in action!',
    '{name} is wielding their tools with precision!',
    'Tool time for {name} — executing with purpose!',
  ],
};

/**
 * Pick a random template for the given trigger type.
 * Returns null if the trigger type is unknown.
 */
export function pickTemplate(trigger) {
  const pool = TEMPLATES[trigger];
  if (!pool || pool.length === 0) return null;
  return pool[Math.floor(Math.random() * pool.length)];
}

/**
 * Fill template placeholders with provided values.
 * Supports {name} and {other} substitutions.
 */
export function fillTemplate(template, vars) {
  return template.replace(/\{(\w+)\}/g, (match, key) => vars[key] || match);
}
