export type StepDefinition = {
  id: string;
  label: string;
  description?: string;
  state?: 'default' | 'success' | 'error' | 'blocked';
};
