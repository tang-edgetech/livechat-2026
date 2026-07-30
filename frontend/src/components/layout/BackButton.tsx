"use client";

import { ArrowLeftOutlined } from "@ant-design/icons";
import { Button } from "antd";
import { usePathname, useRouter } from "next/navigation";

import { getParentRoute } from "@/lib/routes";

export function BackButton() {
  const pathname = usePathname();
  const router = useRouter();
  const parent = getParentRoute(pathname);

  if (parent === pathname) return null;

  return (
    <Button
      type="text"
      icon={<ArrowLeftOutlined />}
      onClick={() => router.push(parent)}
    >
      Back
    </Button>
  );
}
