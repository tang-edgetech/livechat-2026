import type { ReactNode } from "react";
import { Typography } from "antd";

// Shared page-header treatment (icon chip + title + one-line description)
// introduced for the Settings rebuild — reused wherever a top-level page
// needs the same "grand, not just a bare list" framing instead of content
// starting cold with no title at all.
export function PageHeader({
  icon,
  title,
  description,
  extra,
}: {
  icon: ReactNode;
  title: string;
  description?: string;
  extra?: ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="flex items-start gap-3">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-black/10 bg-black/[0.03] text-[17px] dark:border-white/10 dark:bg-white/[0.06]">
          {icon}
        </div>
        <div className="flex flex-col">
          <Typography.Title level={4} style={{ margin: 0 }}>
            {title}
          </Typography.Title>
          {description && (
            <Typography.Text type="secondary" style={{ fontSize: 13.5 }}>
              {description}
            </Typography.Text>
          )}
        </div>
      </div>
      {extra}
    </div>
  );
}
