import type { CapabilityState } from './capabilities';

export enum EventSource {
  HAP = 'hap',
  ADAPTER = 'adapter',
  POLL = 'poll',
  HUB = 'hub',
}

export enum HubEventType {
  DEVICE_STATE_CHANGED = 'device.state.changed',
  DEVICE_ONLINE_CHANGED = 'device.online.changed',
  ADAPTER_ERROR = 'adapter.error',
}

export interface DeviceStateChangedEvent {
  type: HubEventType.DEVICE_STATE_CHANGED;
  deviceId: string;
  capability: CapabilityState;
  previousState: CapabilityState | null;
  source: EventSource;
  timestamp: Date;
}

export interface DeviceOnlineChangedEvent {
  type: HubEventType.DEVICE_ONLINE_CHANGED;
  deviceId: string;
  online: boolean;
  timestamp: Date;
}

export interface AdapterErrorEvent {
  type: HubEventType.ADAPTER_ERROR;
  adapterId: string;
  deviceId?: string;
  error: string;
  timestamp: Date;
}

export type HubEvent =
  | DeviceStateChangedEvent
  | DeviceOnlineChangedEvent
  | AdapterErrorEvent;
