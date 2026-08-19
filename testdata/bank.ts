// Sample exercise for uplift: a small banking domain with the type aliases
// convention (uint8/int64 as number aliases) so the Go output has real
// integer types.

type uint8 = number;
type int64 = number;
type Money = int64;

interface Account {
  id: string;
  owner: string;
  balance: Money;
  frozen: boolean;
}

type AccountStatus = "active" | "frozen" | "closed";

enum AccountKind { Checking, Savings = 5, Credit }

class Bank {
  accounts: Account[] = [];

  constructor(private name: string) {}

  open(owner: string, initial: Money): Account {
    const acc: Account = {
      id: this.name + "-" + owner,
      owner: owner,
      balance: initial,
      frozen: false,
    };
    this.accounts.push(acc);
    return acc;
  }

  total(): Money {
    let sum: Money = 0;
    for (const a of this.accounts) {
      sum += a.balance;
    }
    return sum;
  }

  describe(): string {
    return `Bank ${this.name} has ${this.accounts.length} accounts`;
  }
}

function max(a: number, b: number): number {
  if (a > b) {
    return a;
  } else {
    return b;
  }
}

function statusLabel(status: AccountStatus): string {
  switch (status) {
    case "active":
      return "Active";
    case "frozen":
      return "Frozen";
    default:
      return "Unknown";
  }
}
