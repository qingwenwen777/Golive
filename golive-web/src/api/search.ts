import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { http } from '@/lib/axios';
import type { ChannelPost } from '@/api/posts';
import type { AppointmentItem } from '@/api/room';
import type { Stream } from '@/types/stream';

export interface SearchCreator {
  id: string;
  channelId: string;
  username?: string;
  name: string;
  avatar: string;
  cover?: string;
  verified: boolean;
  subscriberCount: number;
  live: boolean;
  liveRoomId?: string;
  lastLiveAt?: string;
  lastTitle?: string;
  following: boolean;
  self: boolean;
}

export interface SearchResultsResp {
  query: string;
  creators: SearchCreator[];
  live: Stream[];
  replays: Stream[];
  appointments: AppointmentItem[];
  posts: ChannelPost[];
  total: number;
}

export interface SearchSuggestion {
  value: string;
  type: 'query' | 'creator' | 'live' | 'replay' | 'appointment' | 'post' | string;
  label?: string;
}

export interface SearchSuggestionResp {
  query: string;
  items: SearchSuggestion[];
}

export function useSearchResults(query: string, size = 8, enabled = true) {
  const q = query.trim();
  return useQuery<SearchResultsResp, Error>({
    queryKey: ['search-results', q, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<SearchResultsResp>('/rooms/search', {
        params: { q, size },
        signal,
      });
      return data;
    },
    enabled: enabled && q.length > 0,
    staleTime: 20_000,
    placeholderData: keepPreviousData,
    retry: 1,
  });
}

export function useSearchSuggestions(query: string, enabled = true, size = 12) {
  const q = query.trim();
  return useQuery<SearchSuggestionResp, Error>({
    queryKey: ['search-suggestions', q, size],
    queryFn: async ({ signal }) => {
      const { data } = await http.get<SearchSuggestionResp>('/rooms/search/suggestions', {
        params: { q, size },
        signal,
      });
      return data;
    },
    enabled: enabled && q.length > 0,
    staleTime: 30_000,
    retry: 1,
  });
}
