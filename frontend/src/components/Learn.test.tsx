import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Learn } from "./Learn";

describe("Learn", () => {
  it("renders every theoretical-foundations section", () => {
    render(<Learn />);

    expect(screen.getByRole("heading", { name: "Theoretical Foundations" })).toBeInTheDocument();
    for (const name of [
      "Consensus Theory",
      "Replicated State Machines",
      "CAP Theorem",
      "Quorum / Majority Logic",
      "Consistency Models",
      "Formal Verification & Model Checking",
      "The Asynchronous Model",
    ]) {
      expect(screen.getByRole("heading", { name })).toBeInTheDocument();
    }
  });

  it("names the consensus safety and liveness properties", () => {
    render(<Learn />);

    expect(
      screen.getByRole("heading", { name: "Safety — What Must Never Happen" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Liveness — What Must Eventually Happen" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Election Safety/)).toBeInTheDocument();
    expect(screen.getByText(/Leader Completeness/)).toBeInTheDocument();
  });

  it("calls the store CP and lays out the consistency models", () => {
    render(<Learn />);

    expect(
      within(screen.getByRole("img", { name: /CAP triangle/i })).getByText("CP"),
    ).toBeInTheDocument();
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Linearizability (strongest)" })).toBeInTheDocument();
    expect(
      screen.getByRole("cell", { name: "Eventual consistency (weakest)" }),
    ).toBeInTheDocument();
  });

  it("renders the CAP and quorum diagrams", () => {
    render(<Learn />);

    expect(screen.getByRole("img", { name: /CAP triangle/i })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: /Quorum intersection/i })).toBeInTheDocument();
  });
});
