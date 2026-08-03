"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { Spin } from "antd";

import { apiGet } from "@/lib/api";
import { BotFlowEditor } from "@/components/settings/botflow/BotFlowEditor";
import type { BotFlow } from "@/lib/automationTypes";

export default function EditBotFlowPage() {
  const { id } = useParams<{ id: string }>();
  const [flow, setFlow] = useState<BotFlow | null>(null);

  useEffect(() => {
    apiGet<{ botFlows: BotFlow[] }>("/api/bot-flows").then((res) => {
      setFlow(res.botFlows.find((f) => String(f.id) === id) ?? null);
    });
  }, [id]);

  if (!flow) {
    return (
      <div className="flex h-40 items-center justify-center">
        <Spin />
      </div>
    );
  }

  return <BotFlowEditor existing={flow} />;
}
