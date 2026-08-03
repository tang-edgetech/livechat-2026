"use client";

import { useState } from "react";
import { Segmented } from "antd";

import { FilesRulesSection } from "@/components/settings/FilesRulesSection";
import { FilesLibrarySection } from "@/components/settings/FilesLibrarySection";

// Files (overview.md §6.8) — Rules (global default + per-merchant
// overrides) and Library (what's actually been uploaded) used to be one
// rules-only form; Library is new.
export function FilesTab() {
  const [section, setSection] = useState<"rules" | "library">("rules");

  return (
    <div className="flex flex-col gap-4">
      <Segmented
        value={section}
        onChange={(v) => setSection(v as "rules" | "library")}
        options={[
          { value: "rules", label: "Rules" },
          { value: "library", label: "Library" },
        ]}
      />
      {section === "rules" ? <FilesRulesSection /> : <FilesLibrarySection />}
    </div>
  );
}
