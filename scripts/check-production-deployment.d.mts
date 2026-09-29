export interface ProductionEnvironment {
  IMAGE_TAG: string;
  WEB_IMAGE: string;
  API_IMAGE: string;
  APP_DOMAIN: string;
  POSTGRES_DB: string;
  POSTGRES_USER: string;
  POSTGRES_PASSWORD: string;
}

export function validateProductionEnvironment(
  environment: ProductionEnvironment,
): void;

export function validateProductionTopology(compose: unknown): void;
