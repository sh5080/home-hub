export enum CapabilityKind {
  ON_OFF = 'onOff',
  THERMOSTAT = 'thermostat',
  FAN_SPEED = 'fanSpeed',
  BRIGHTNESS = 'brightness',
  COLOR_TEMP = 'colorTemp',
  WINDOW_COVERING = 'windowCovering',
}

export interface OnOffState {
  kind: CapabilityKind.ON_OFF;
  value: boolean;
}

export enum ThermostatMode {
  OFF = 'off',
  COOL = 'cool',
  HEAT = 'heat',
  AUTO = 'auto',
  FAN = 'fan',
  DRY = 'dry',
}

export enum TemperatureUnit {
  CELSIUS = 'C',
  FAHRENHEIT = 'F',
}

export interface ThermostatState {
  kind: CapabilityKind.THERMOSTAT;
  mode: ThermostatMode;
  currentTemp: number;
  targetTemp: number;
  unit: TemperatureUnit;
}

export enum FanSpeedLevel {
  AUTO = 'auto',
  LOW = 'low',
  MID = 'mid',
  HIGH = 'high',
}

export interface FanSpeedState {
  kind: CapabilityKind.FAN_SPEED;
  level: FanSpeedLevel;
}

export interface BrightnessState {
  kind: CapabilityKind.BRIGHTNESS;
  value: number; // 0-100
}

export interface ColorTempState {
  kind: CapabilityKind.COLOR_TEMP;
  kelvin: number;
}

export interface WindowCoveringState {
  kind: CapabilityKind.WINDOW_COVERING;
  position: number; // 0 = closed, 100 = open
}

export type CapabilityState =
  | OnOffState
  | ThermostatState
  | FanSpeedState
  | BrightnessState
  | ColorTempState
  | WindowCoveringState;
