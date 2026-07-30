"use client";

import { Modal } from "antd";
import { ExclamationCircleFilled } from "@ant-design/icons";

type ConfirmOptions = {
  title: string;
  content?: React.ReactNode;
  okText?: string;
  danger?: boolean;
  onConfirm: () => void | Promise<void>;
};

// Shared confirmation popup required before Logout, Save, Delete, Bulk
// Edit apply, Export, etc. (overview.md §6.0). One helper, one look and
// feel, called from anywhere instead of each screen rolling its own.
export function confirmAction({ title, content, okText = "Confirm", danger, onConfirm }: ConfirmOptions) {
  Modal.confirm({
    title,
    content,
    icon: <ExclamationCircleFilled />,
    okText,
    okButtonProps: danger ? { danger: true } : undefined,
    cancelText: "Cancel",
    onOk: onConfirm,
  });
}
