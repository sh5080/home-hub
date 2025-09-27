import type { CapabilityState } from './capabilities';
import { type CapabilityKind } from './capabilities';

export enum DeviceType {
  AC = 'ac',
  LIGHT = 'light',
  BLIND = 'blind',
  BOILER = 'boiler',
  SWITCH = 'switch',
  SENSOR = 'sensor',
}

export interface Device {
  id: string;
  name: string;
  adapterId: string;
  type: DeviceType;
  capabilities: CapabilityKind[];
  online: boolean;
}

export interface DeviceWithState extends Device {
  states: Map<CapabilityKind, CapabilityState>;
}
