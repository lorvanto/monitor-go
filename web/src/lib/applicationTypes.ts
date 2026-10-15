import { localizedLabels } from "../i18n";
export interface Application {
  id: string;
  slug: string;
  name: string;
  description: string;
  enabled: boolean;
  version: number;
  createdAt: number;
  updatedAt: number;
  activeKeys: number;
  releases: number;
}
export interface ApplicationKey {
  id: string;
  applicationId: string;
  name: string;
  prefix: string;
  scopes: string[];
  expiresAt: number;
  lastUsedAt: number;
  createdAt: number;
  revokedAt: number;
  version: number;
}
export interface Release {
  id: string;
  applicationId: string;
  applicationName: string;
  applicationEnabled: boolean;
  name: string;
  versionName: string;
  platform: string;
  arch: string;
  source: string;
  fileId: string;
  fileName: string;
  size: number;
  sha256: string;
  downloadUrl: string;
  notes: string;
  state: string;
  everPublished: boolean;
  downloadCount: number;
  version: number;
  createdAt: number;
  updatedAt: number;
  publishedAt: number;
}
export const platforms: Record<string, string> = localizedLabels({
  windows: "Windows",
  macos: "macOS",
  linux: "Linux",
  android: "Android",
  ios: "iOS",
  web: "Web",
  other: "其他",
});
export const releaseStates: Record<string, string> = localizedLabels({
  draft: "草稿",
  published: "已发布",
  withdrawn: "已下架",
});
export { dateTime } from "./format";
