let permissionGranted = false;

export async function requestPermission() {
  if (!('Notification' in window)) return;
  if (Notification.permission === 'granted') {
    permissionGranted = true;
    return;
  }
  if (Notification.permission !== 'denied') {
    const result = await Notification.requestPermission();
    permissionGranted = result === 'granted';
  }
}

export function notifyCompletion(name, activity) {
  if (!permissionGranted) return;

  const isError = activity === 'errored';
  const title = isError ? `Session Error: ${name}` : `Session Complete: ${name}`;
  const body = isError
    ? `${name} encountered an error`
    : `${name} finished successfully`;

  try {
    new Notification(title, { body, tag: `race-${name}` });
  } catch {
    // Notifications may not be available in all contexts
  }
}
