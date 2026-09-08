declare global {
  interface Window {
    go?: { main: { App: Record<string, (...args: any[]) => Promise<any>> } };
    runtime?: {
      EventsOn: (name: string, cb: (...args: any[]) => void) => () => void;
    };
  }
}
export const desktop = !!window.go;
export async function invoke<T>(method: string, ...args: any[]): Promise<T> {
  if (window.go) return window.go.main.App[method](...args);
  const response = await fetch("/api/" + method, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-ColorNinja": "studio" },
    body: JSON.stringify(args),
  });
  if (!response.ok) throw new Error(await response.text());
  const body = await response.json();
  if (body.error) throw new Error(body.error);
  return body.result as T;
}
export function on(name: string, cb: (...args: any[]) => void) {
  return window.runtime?.EventsOn(name, cb) ?? (() => {});
}
