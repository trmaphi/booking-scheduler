export interface BoundaryCheckOptions {
  requireModuleRoots?: boolean;
}

export function findBoundaryViolations(
  root: string,
  options?: BoundaryCheckOptions,
): Promise<string[]>;
