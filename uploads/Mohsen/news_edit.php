<?
admin_check_login();

$my_sql_info = $class_db->sql_query("SELECT * FROM `md_news` where `id`='".(int)$_GET['id']."' AND `adminid`='".$my_config_r['id']."' ");
$my_sql_info_r = $class_db->sql_fetchrow( $my_sql_info );
?>

 <script src="/js/editor.js" type="text/javascript"></script>
<b> ويرايش خبر </b>
<div class="my_error_error not_show" id="ajax_error" ></div>
<form id="md_form" ajaxurl="ajax.telir?page=<?=$get_page?>" ajax="ok">
<input type="hidden" id="md_edit_id" ajax_require="ok" ajax_error="مشخصات رکورد حذف شده است" value="<?=$_GET['id']?>" />
<table border="0" width="100%" dir="rtl" cellspacing="0" cellpadding="0">
  <tr>
    <td height="40"  width="100px">عنوان :</td>
    <td><input type="text" id="title" size="40" dir="rtl" lang="fa" value="<?=$class_db->undo_escape($my_sql_info_r['title'])?>" /></td>
  </tr>
  <tr>
    <td height="40"  width="100px">متن :</td>
    <td><textarea type="text" id="note" class="editor" ><?=$class_db->undo_escape($my_sql_info_r['content'])?></textarea></td>
  </tr>
  <tr>
    <td height="30">&nbsp;</td>
    <td><button type="submit" class="button_ok" id="button_form">ثبت</button></td>
  </tr>
</table>
</form>
<script>
editor_insert('note');
</script>