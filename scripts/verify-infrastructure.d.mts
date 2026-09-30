export interface InfrastructureStep {
  name: string;
  command: string;
  args: string[];
  env?: Record<string, string>;
}

export const infrastructureSteps: InfrastructureStep[];
export function runInfrastructureSteps(
  steps?: InfrastructureStep[],
  runner?: (step: InfrastructureStep) => void,
): void;
