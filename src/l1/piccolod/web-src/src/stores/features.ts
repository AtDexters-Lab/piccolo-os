import { writable } from 'svelte/store';

export type FeatureId = 'remote' | 'clustering' | 'federation' | 'certificate-inventory';

export type FeatureRecord = {
  id: FeatureId;
  name: string;
  description: string;
  enabled: boolean;
  docsUrl?: string;
};

const STORAGE_KEY = 'piccolo_features';

const defaultFeatures: FeatureRecord[] = [
  {
    id: 'remote',
    name: 'Remote access (Nexus)',
    description: 'Publish Piccolo through Nexus with device-terminated TLS.',
    enabled: false
  },
  {
    id: 'clustering',
    name: 'Multi-device clustering',
    description: 'Join additional Piccolo devices to form a resilient cluster.',
    enabled: false
  },
  {
    id: 'federation',
    name: 'Storage federation',
    description: 'Export cold data to remote peers for long-term durability.',
    enabled: false
  },
  {
    id: 'certificate-inventory',
    name: 'Certificate inventory',
    description: 'Track issued certificates, renewal windows, and alias domains.',
    enabled: false
  }
];

function loadFeatures(): FeatureRecord[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return defaultFeatures;
    const parsed = JSON.parse(raw) as FeatureRecord[];
    const merged = defaultFeatures.map((feature) => {
      const existing = parsed.find((record) => record.id === feature.id);
      return existing ? { ...feature, enabled: !!existing.enabled } : feature;
    });
    return merged;
  } catch {
    return defaultFeatures;
  }
}

function persistFeatures(features: FeatureRecord[]) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(features));
  } catch {
    /* ignore */
  }
}

const featuresStore = writable<FeatureRecord[]>(defaultFeatures, (set) => {
  if (typeof window !== 'undefined') {
    set(loadFeatures());
  }
  return () => undefined;
});

featuresStore.subscribe((value) => {
  if (typeof window !== 'undefined') {
    persistFeatures(value);
  }
});

export function enableFeature(id: FeatureId) {
  featuresStore.update((list) =>
    list.map((feature) => (feature.id === id ? { ...feature, enabled: true } : feature))
  );
}

export function disableFeature(id: FeatureId) {
  featuresStore.update((list) =>
    list.map((feature) => (feature.id === id ? { ...feature, enabled: false } : feature))
  );
}

export { featuresStore };
