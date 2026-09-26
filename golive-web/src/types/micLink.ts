export type MicLinkEligibility = 'all' | 'followers' | 'fans' | 'fans_level';
export type MicLinkMyStatus = 'none' | 'pending' | 'on_air';

export interface MicLinkRequest {
  userId: string;
  name: string;
  avatar?: string;
  createdAt: number;
}

export interface MicLinkGuest {
  userId: string;
  name: string;
  avatar?: string;
  muted: boolean;
  joinedAt: number;
}

export interface MicLinkView {
  enabled: boolean;
  eligibility: MicLinkEligibility;
  minFanLevel: number;
  slotMax: number;
  roster: MicLinkGuest[];
  requests?: MicLinkRequest[];
  isOwner: boolean;
  myStatus: MicLinkMyStatus;
  myMuted: boolean;
  // Present only for the caller while on air; authorizes their WHIP publish.
  myPublishToken?: string;
}
