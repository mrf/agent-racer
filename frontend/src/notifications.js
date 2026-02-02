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
  const isLost = activity === 'lost';

  let title, body;
  if (isError) {
    title = `Session Error: ${name}`;
    body = `${name} encountered an error`;
  } else if (isLost) {
    title = `Session Lost: ${name}`;
    body = `${name} disappeared or crashed`;
  } else {
    title = `Session Complete: ${name}`;
    body = `${name} finished successfully`;
  }

  try {
    new Notification(title, { body, tag: `race-${name}` });
  } catch {
    // Notifications may not be available in all contexts
  }
}
