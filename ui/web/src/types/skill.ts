export interface SkillInfo {
  id?: string;
  name: string;
  slug?: string;
  description: string;
  source: string;
}

export interface SkillWithGrant {
  id: string;
  name: string;
  slug: string;
  description: string;
  visibility: string;
  version: number;
  granted: boolean;
  pinned_version?: number;
}
