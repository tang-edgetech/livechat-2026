import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { AntdRegistry } from "@ant-design/nextjs-registry";

import "./globals.css";
import { AuthProvider } from "@/context/AuthContext";
import { IdleTimeoutModal } from "@/components/modals/IdleTimeoutModal";
import { ThemedApp } from "@/components/ThemedApp";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "LiveChat",
  description: "Customer-care / livechat support platform",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <AntdRegistry>
          <AuthProvider>
            <ThemedApp>
              {children}
              <IdleTimeoutModal />
            </ThemedApp>
          </AuthProvider>
        </AntdRegistry>
      </body>
    </html>
  );
}
