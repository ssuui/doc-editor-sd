export async function request<T>(url: string, init: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem("token") || "";
  const headers = new Headers(init.headers || {});
  headers.set("Authorization", `Bearer ${token}`);
  if (!headers.has("Content-Type") && init.body) headers.set("Content-Type", "application/json");
  const res = await fetch(url, { ...init, headers });
  const json = await res.json();
  if (json.code === 1001) {
    localStorage.removeItem("token");
    alert("登录失效或已在其他设备登录");
    location.href = "/login.html";
    throw new Error(json.msg);
  }
  if (json.code !== 0) {
    alert(json.msg || "请求失败");
    throw new Error(json.msg || "request failed");
  }
  return json.data as T;
}
