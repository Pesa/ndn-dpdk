type EalLCoreConfig = {
  cores?: number[];
  coresPerNuma?: Record<number, number>;
} | {
  lcoreFlags?: string;
};

type EalMemoryConfig = {
  memChannels?: number;
  memPerNuma?: Record<number, number>;
  filePrefix?: string;
  disableHugeUnlink?: boolean;
} | {
  memFlags?: string;
};

type EalDeviceConfig = {
  drivers?: string[];
  pciDevices?: string[];
  allPciDevices?: boolean;
  virtualDevices?: string[];
} | {
  deviceFlags?: string;
};

export type EalConfig =
  (EalLCoreConfig & EalMemoryConfig & EalDeviceConfig & { extraFlags?: string }) |
  { flags?: string };

/**
 * DPDK logical core number.
 * @TJS-type integer
 * @minimum 0
 */
export type LCore = number;

export type LCoreAllocConfig<K extends string = string> = Partial<Record<K, LCoreAllocConfig.Role>>;

export namespace LCoreAllocConfig {
  export interface Role {
    lcores?: LCore[];
    onNuma?: Record<number, number>;
    eachNuma?: number;
  }
}

export interface PktmbufPoolConfig {
  capacity?: number;
  privSize?: number;
  dataroom?: number;
}

export type PktmbufPoolTemplateUpdates<K extends string = string> = Partial<Record<K, PktmbufPoolConfig>>;
