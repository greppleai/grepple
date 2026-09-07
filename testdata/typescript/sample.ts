import { logger } from "./logger";

type User = {
  id: string;
  name: string;
};

export function formatUser(user: User): string {
  const trimmedName = user.name.trim();
  logger.info("formatUser", { id: user.id });
  return `${trimmedName} (${user.id})`;
}

export class UserService {
  constructor(private readonly users: User[]) {}

  findUser(id: string): User | undefined {
    return this.users.find((user) => user.id === id);
  }

  printUser(id: string): void {
    const user = this.findUser(id);
    if (!user) return;
    console.log(formatUser(user));
  }
}
