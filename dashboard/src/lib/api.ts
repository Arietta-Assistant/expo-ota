import { getRefreshToken, getToken, logout, setTokens } from '@/lib/auth.ts';

export class ApiClient {
  private baseUrl: string;

  constructor() {
    // @ts-expect-error - window.env is defined in index.html
    this.baseUrl = window?.env?.VITE_OTA_API_URL || import.meta.env.VITE_OTA_API_URL;
    if (!this.baseUrl) {
      throw new Error('Missing VITE_OTA_API_URL environment variable');
    }
  }

  private populateHeaders(headers: Headers) {
    const token = getToken();
    if (token) {
      headers.set('Authorization', `Bearer ${token}`);
    }
  }
  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const headers = new Headers(options.headers);
    this.populateHeaders(headers);

    const response = await fetch(url, { ...options, headers });
    const refreshToken = getRefreshToken();
    if (response.status === 401 && refreshToken) {
      await this.refreshTokens(refreshToken);
      return this.request<T>(endpoint, options);
    }

    if (!response.ok) {
      throw new Error(`HTTP error! Status: ${response.status}`);
    }

    return response.json() as Promise<T>;
  }

  private async refreshTokens(refreshToken: string) {
    try {
      const form = new URLSearchParams();
      form.append('refreshToken', refreshToken);
      const response = await fetch(`${this.baseUrl}/auth/refreshToken`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: form.toString(),
      });

      if (!response.ok) {
        throw new Error('Failed to refresh token');
      }

      const data = await response.json();
      setTokens(data.token, data.refreshToken);

      localStorage.setItem('accessToken', data.token);
      localStorage.setItem('refreshToken', data.refreshToken);
    } catch (error) {
      console.error('Failed to refresh token:', error);
      logout();
    }
  }

  public async login(password: string) {
    const form = new URLSearchParams();
    form.append('password', password);
    return this.request<{ token: string; refreshToken: string }>(`/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: form.toString(),
    });
  }

  public async getBranches() {
    return this.request<
      {
        branchName: string;
        releaseChannel?: string | null;
      }[]
    >('/api/branches', {
      method: 'GET',
    });
  }
  public async getRuntimeVersions(branch: string) {
    return this.request<
      {
        runtimeVersion: string;
        lastUpdatedAt: string;
        createdAt: string;
        numberOfUpdates: number;
      }[]
    >(`/api/runtime-versions/${branch}`, {
      method: 'GET',
    });
  }
  public async getUpdates(branch: string, runtimeVersion: string) {
    return this.request<
      {
        updateUUID: string;
        createdAt: string;
        updateId: string;
        platform: string;
        commitHash: string;
      }[]
    >(`/api/updates/${branch}/${runtimeVersion}`, {
      method: 'GET',
    });
  }
  public async getSettings() {
    return this.request<{
      BASE_URL: string;
      EXPO_APP_ID: string;
      EXPO_ACCESS_TOKEN: string;
      CACHE_MODE: string;
      REDIS_HOST: string;
      REDIS_PORT: string;
      STORAGE_MODE: string;
      S3_BUCKET_NAME: string;
      LOCAL_BUCKET_BASE_PATH: string;
      KEYS_STORAGE_TYPE: string;
      AWSSM_EXPO_PUBLIC_KEY_SECRET_ID: string;
      AWSSM_EXPO_PRIVATE_KEY_SECRET_ID: string;
      PUBLIC_EXPO_KEY_B64: string;
      PUBLIC_LOCAL_EXPO_KEY_PATH: string;
      PRIVATE_LOCAL_EXPO_KEY_PATH: string;
      AWS_REGION: string;
      AWS_ACCESS_KEY_ID: string;
      CLOUDFRONT_DOMAIN: string;
      CLOUDFRONT_KEY_PAIR_ID: string;
      CLOUDFRONT_PRIVATE_KEY_B64: string;
      AWSSM_CLOUDFRONT_PRIVATE_KEY_SECRET_ID: string;
      PRIVATE_LOCAL_CLOUDFRONT_KEY_PATH: string;
      PROMETHEUS_ENABLED: string;
    }>(`/api/settings`, {
      method: 'GET',
    });
  }
}

export const api = new ApiClient();
