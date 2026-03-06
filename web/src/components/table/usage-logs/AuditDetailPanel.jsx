import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Space, Spin, Tag, Typography } from '@douyinfe/semi-ui';
import { API, getLogOther } from '../../../helpers';

const { Text } = Typography;

function formatJsonText(value) {
  if (value === undefined || value === null || value === '') {
    return '';
  }
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) {
      return '';
    }
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch (e) {
      return trimmed;
    }
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch (e) {
    return String(value);
  }
}

const renderJsonBlock = (value, t) => {
  const text = formatJsonText(value);
  if (!text) {
    return <Text type='tertiary'>{t('无审计数据')}</Text>;
  }
  return (
    <pre
      style={{
        margin: 0,
        maxWidth: 760,
        maxHeight: 260,
        overflow: 'auto',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        lineHeight: 1.5,
        padding: 8,
        borderRadius: 6,
        backgroundColor: 'var(--semi-color-fill-0)',
      }}
    >
      {text}
    </pre>
  );
};

const AuditDetailPanel = ({ logRecord, t }) => {
  const [loading, setLoading] = useState(false);
  const [auditRecord, setAuditRecord] = useState(null);
  const [errorText, setErrorText] = useState('');

  const requestPath = useMemo(() => {
    if (logRecord?.request_path) {
      return String(logRecord.request_path);
    }
    const other = getLogOther(logRecord?.other);
    if (other?.request_path) {
      return String(other.request_path);
    }
    return '';
  }, [logRecord?.other, logRecord?.request_path]);

  const queryString = useMemo(() => {
    const params = new URLSearchParams();
    if (logRecord?.created_at) {
      params.set('created_at', String(logRecord.created_at));
    }
    if (logRecord?.user_id) {
      params.set('user_id', String(logRecord.user_id));
    }
    if (logRecord?.token_id) {
      params.set('token_id', String(logRecord.token_id));
    }
    if (requestPath) {
      params.set('path', requestPath);
    }
    params.set('method', 'POST');
    return params.toString();
  }, [
    logRecord?.created_at,
    logRecord?.token_id,
    logRecord?.user_id,
    requestPath,
  ]);

  const fetchAudit = useCallback(async () => {
    setLoading(true);
    setErrorText('');
    try {
      let url = '';
      if (logRecord?.request_id) {
        const requestId = encodeURIComponent(String(logRecord.request_id));
        url = `/api/audit/request/${requestId}`;
      } else {
        url = '/api/audit/match';
      }
      if (queryString) {
        url = `${url}?${queryString}`;
      }
      const res = await API.get(url);
      const { success, message, data } = res.data;
      if (!success) {
        setErrorText(message || t('获取审计数据失败'));
        setAuditRecord(null);
      } else if (!data?.found || !data?.record) {
        setAuditRecord(null);
      } else {
        setAuditRecord(data.record);
      }
    } catch (e) {
      setErrorText(t('获取审计数据失败'));
      setAuditRecord(null);
    } finally {
      setLoading(false);
    }
  }, [logRecord?.request_id, queryString, t]);

  useEffect(() => {
    fetchAudit().catch(() => {});
  }, [fetchAudit]);

  if (loading) {
    return (
      <div style={{ minHeight: 48 }}>
        <Spin tip={t('加载详情中...')} />
      </div>
    );
  }

  if (errorText) {
    return (
      <Space vertical align='start' spacing='tight'>
        <Text type='danger'>{errorText}</Text>
        <Button type='tertiary' size='small' onClick={fetchAudit}>
          {t('重试')}
        </Button>
      </Space>
    );
  }

  if (!auditRecord) {
    return <Text type='tertiary'>{t('无审计数据')}</Text>;
  }

  return (
    <Space vertical align='start' spacing='tight'>
      <Space wrap>
        <Tag color='blue'>{`${t('路径')}: ${auditRecord.path || '-'}`}</Tag>
        <Tag color='cyan'>{`${t('模型')}: ${auditRecord.model || '-'}`}</Tag>
        <Tag color='green'>{`${t('状态码')}: ${auditRecord.status_code ?? '-'}`}</Tag>
        <Tag color='orange'>{`${t('耗时(ms)')}: ${auditRecord.latency_ms ?? '-'}`}</Tag>
        <Tag color='purple'>{`${t('stream')}: ${String(Boolean(auditRecord.stream))}`}</Tag>
      </Space>
      <Text>{`${t('客户端 IP')}: ${auditRecord.client_ip || '-'}`}</Text>
      <Text>{`${t('User-Agent')}: ${auditRecord.user_agent || '-'}`}</Text>
      <Text strong>{t('messages')}</Text>
      {renderJsonBlock(auditRecord.messages, t)}
      <Text strong>{t('metadata')}</Text>
      {renderJsonBlock(auditRecord.metadata, t)}
      <Text strong>{t('tools')}</Text>
      {renderJsonBlock(auditRecord.tools, t)}
      <Text strong>{t('tool_choice')}</Text>
      {renderJsonBlock(auditRecord.tool_choice, t)}
      <Text strong>{t('input')}</Text>
      {renderJsonBlock(auditRecord.input, t)}
    </Space>
  );
};

export default AuditDetailPanel;
